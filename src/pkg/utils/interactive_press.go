package utils

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// Interactive press: business bots gate their flows on a tap (reply buttons,
// list menus, template quick replies, native-flow buttons). A typed reply with
// the same words is usually rejected ("No te entendi"), because the bot keys on
// the selected id, not the text. InteractiveOptions lists what can be tapped on
// an inbound prompt; BuildInteractiveResponse builds the response proto a phone
// sends when the option is tapped.

const (
	InteractiveKindButtons    = "buttons"
	InteractiveKindList       = "list"
	InteractiveKindTemplate   = "template"
	InteractiveKindNativeFlow = "native_flow"
)

// InteractiveOption is one tappable choice on an inbound interactive prompt.
// Index is 1-based and stable for a given message.
type InteractiveOption struct {
	Index       int    `json:"index"`
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Section     string `json:"section,omitempty"`
	// FlowName is the native-flow button name (quick_reply, single_select, ...).
	FlowName string `json:"flow_name,omitempty"`
	// Pressable is false for buttons a phone resolves locally (url, call,
	// copy code): there is no reply to send for those.
	Pressable bool   `json:"pressable"`
	URL       string `json:"url,omitempty"`
	Phone     string `json:"phone_number,omitempty"`
	Code      string `json:"code,omitempty"`

	templateIndex uint32
}

// InteractivePrompt is the readable form of an interactive prompt.
type InteractivePrompt struct {
	Kind    string              `json:"kind"`
	Title   string              `json:"title,omitempty"`
	Body    string              `json:"body,omitempty"`
	Footer  string              `json:"footer,omitempty"`
	Button  string              `json:"button_text,omitempty"` // list opener label
	Options []InteractiveOption `json:"options"`
}

// UnwrapInteractive strips the container messages business senders use around
// interactive content.
func UnwrapInteractive(msg *waE2E.Message) *waE2E.Message {
	for i := 0; msg != nil && i < 6; i++ {
		switch {
		case msg.GetViewOnceMessage().GetMessage() != nil:
			msg = msg.GetViewOnceMessage().GetMessage()
		case msg.GetViewOnceMessageV2().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2().GetMessage()
		case msg.GetViewOnceMessageV2Extension().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2Extension().GetMessage()
		case msg.GetEphemeralMessage().GetMessage() != nil:
			msg = msg.GetEphemeralMessage().GetMessage()
		case msg.GetDocumentWithCaptionMessage().GetMessage() != nil:
			msg = msg.GetDocumentWithCaptionMessage().GetMessage()
		case msg.GetDeviceSentMessage().GetMessage() != nil:
			msg = msg.GetDeviceSentMessage().GetMessage()
		default:
			return msg
		}
	}
	return msg
}

// IsInteractivePrompt reports whether msg carries something a user can tap.
func IsInteractivePrompt(msg *waE2E.Message) bool {
	p := ParseInteractivePrompt(msg)
	return p != nil && len(p.Options) > 0
}

// ParseInteractivePrompt returns the prompt and its options, or nil.
func ParseInteractivePrompt(msg *waE2E.Message) *InteractivePrompt {
	msg = UnwrapInteractive(msg)
	if msg == nil {
		return nil
	}
	var p *InteractivePrompt
	switch {
	case msg.GetButtonsMessage() != nil:
		p = buttonsPrompt(msg.GetButtonsMessage())
	case msg.GetListMessage() != nil:
		p = listPrompt(msg.GetListMessage())
	case templateMessage(msg) != nil:
		tm := templateMessage(msg)
		if im := tm.GetInteractiveMessageTemplate(); im != nil {
			p = nativeFlowPrompt(im)
		} else {
			p = templatePrompt(tm)
		}
	case msg.GetInteractiveMessage() != nil:
		p = nativeFlowPrompt(msg.GetInteractiveMessage())
	}
	if p == nil {
		return nil
	}
	for i := range p.Options {
		p.Options[i].Index = i + 1
	}
	return p
}

func buttonsPrompt(bm *waE2E.ButtonsMessage) *InteractivePrompt {
	p := &InteractivePrompt{Kind: InteractiveKindButtons, Title: bm.GetText(), Body: bm.GetContentText(), Footer: bm.GetFooterText()}
	for _, b := range bm.GetButtons() {
		label := b.GetButtonText().GetDisplayText()
		if info := b.GetNativeFlowInfo(); info != nil {
			nb := nativeFlowAsButton(info.GetName(), info.GetParamsJSON(), b.GetButtonID())
			label = cmp.Or(label, nb.Text)
		}
		p.Options = append(p.Options, InteractiveOption{Kind: InteractiveKindButtons, ID: b.GetButtonID(), Label: label, Pressable: true})
	}
	return p
}

func listPrompt(lm *waE2E.ListMessage) *InteractivePrompt {
	p := &InteractivePrompt{Kind: InteractiveKindList, Title: lm.GetTitle(), Body: lm.GetDescription(), Footer: lm.GetFooterText(), Button: lm.GetButtonText()}
	for _, s := range lm.GetSections() {
		for _, r := range s.GetRows() {
			p.Options = append(p.Options, InteractiveOption{
				Kind: InteractiveKindList, ID: r.GetRowID(), Label: r.GetTitle(),
				Description: r.GetDescription(), Section: s.GetTitle(), Pressable: true,
			})
		}
	}
	return p
}

func templatePrompt(tm *waE2E.TemplateMessage) *InteractivePrompt {
	h := tm.GetHydratedTemplate()
	if h == nil {
		h = tm.GetHydratedFourRowTemplate()
	}
	if h == nil {
		return nil
	}
	p := &InteractivePrompt{Kind: InteractiveKindTemplate, Title: h.GetHydratedTitleText(), Body: h.GetHydratedContentText(), Footer: h.GetHydratedFooterText()}
	for pos, b := range h.GetHydratedButtons() {
		bb, ok := hydratedTemplateButton(b)
		if !ok {
			continue
		}
		idx := b.GetIndex()
		if b.Index == nil {
			idx = uint32(pos)
		}
		p.Options = append(p.Options, InteractiveOption{
			Kind: InteractiveKindTemplate, ID: bb.ID, Label: bb.Text, Pressable: bb.Type == "quick_reply",
			URL: bb.URL, Phone: bb.Phone, templateIndex: idx,
		})
	}
	return p
}

func nativeFlowPrompt(im *waE2E.InteractiveMessage) *InteractivePrompt {
	header := im.GetHeader()
	p := &InteractivePrompt{Kind: InteractiveKindNativeFlow, Title: header.GetTitle(), Body: im.GetBody().GetText(), Footer: im.GetFooter().GetText()}
	for _, b := range im.GetNativeFlowMessage().GetButtons() {
		nb := nativeFlowAsButton(b.GetName(), b.GetButtonParamsJSON(), "")
		name := b.GetName()
		if len(nb.Rows) > 0 {
			p.Button = cmp.Or(p.Button, nb.Text)
			for _, r := range nb.Rows {
				p.Options = append(p.Options, InteractiveOption{
					Kind: InteractiveKindNativeFlow, ID: r.ID, Label: r.Title, Description: r.Description,
					Section: nb.Text, FlowName: name, Pressable: true,
				})
			}
			continue
		}
		local := nb.Type == "url" || nb.Type == "call" || nb.Type == "copy"
		p.Options = append(p.Options, InteractiveOption{
			Kind: InteractiveKindNativeFlow, ID: nb.ID, Label: nb.Text, FlowName: name,
			Pressable: !local, URL: nb.URL, Phone: nb.Phone, Code: nb.Code,
		})
	}
	return p
}

var ErrInteractiveOptionNotFound = errors.New("interactive option not found")

// SelectInteractiveOption picks an option by id, then 1-based index, then a
// case-insensitive label match.
func SelectInteractiveOption(options []InteractiveOption, id string, index int, label string) (InteractiveOption, error) {
	if id != "" {
		for _, o := range options {
			if o.ID == id {
				return o, nil
			}
		}
		return InteractiveOption{}, fmt.Errorf("%w: no option with id %q", ErrInteractiveOptionNotFound, id)
	}
	if index > 0 {
		for _, o := range options {
			if o.Index == index {
				return o, nil
			}
		}
		return InteractiveOption{}, fmt.Errorf("%w: no option at index %d (have %d)", ErrInteractiveOptionNotFound, index, len(options))
	}
	if label != "" {
		want := strings.TrimSpace(strings.ToLower(label))
		for _, o := range options {
			if strings.TrimSpace(strings.ToLower(o.Label)) == want {
				return o, nil
			}
		}
		return InteractiveOption{}, fmt.Errorf("%w: no option labelled %q", ErrInteractiveOptionNotFound, label)
	}
	return InteractiveOption{}, fmt.Errorf("%w: give option_id, index or label", ErrInteractiveOptionNotFound)
}

// PressOverrides lets a caller force the response shape when a bot expects a
// variant other than the one its prompt implies. Zero values mean "derive".
type PressOverrides struct {
	ResponseKind string // buttons, list, template, native_flow
	FlowName     string // native-flow response name
	ParamsJSON   string // native-flow response params
}

// BuildInteractiveResponse builds the message a phone sends when opt is tapped
// on prompt (the original inbound message), quoting it.
func BuildInteractiveResponse(prompt *waE2E.Message, opt InteractiveOption, stanzaID, participant string, ov PressOverrides) (*waE2E.Message, error) {
	if !opt.Pressable && ov.ResponseKind == "" {
		return nil, fmt.Errorf("option %d (%q) is a %s button the phone handles locally; there is nothing to send", opt.Index, opt.Label, cmp.Or(opt.FlowName, opt.Kind))
	}
	ctxInfo := &waE2E.ContextInfo{StanzaID: proto.String(stanzaID), QuotedMessage: UnwrapInteractive(prompt)}
	if participant != "" {
		ctxInfo.Participant = proto.String(participant)
	}
	switch cmp.Or(ov.ResponseKind, opt.Kind) {
	case InteractiveKindButtons:
		return &waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
			SelectedButtonID: proto.String(opt.ID),
			Response:         &waE2E.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: opt.Label},
			Type:             waE2E.ButtonsResponseMessage_DISPLAY_TEXT.Enum(),
			ContextInfo:      ctxInfo,
		}}, nil
	case InteractiveKindList:
		r := &waE2E.ListResponseMessage{
			Title:             proto.String(opt.Label),
			ListType:          waE2E.ListResponseMessage_SINGLE_SELECT.Enum(),
			SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{SelectedRowID: proto.String(opt.ID)},
			ContextInfo:       ctxInfo,
		}
		if opt.Description != "" {
			r.Description = proto.String(opt.Description)
		}
		return &waE2E.Message{ListResponseMessage: r}, nil
	case InteractiveKindTemplate:
		return &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{
			SelectedID:          proto.String(opt.ID),
			SelectedDisplayText: proto.String(opt.Label),
			SelectedIndex:       proto.Uint32(opt.templateIndex),
			ContextInfo:         ctxInfo,
		}}, nil
	case InteractiveKindNativeFlow:
		name := cmp.Or(ov.FlowName, opt.FlowName, "quick_reply")
		params := ov.ParamsJSON
		if params == "" {
			p := map[string]string{"id": opt.ID}
			if opt.Description != "" {
				p["description"] = opt.Description
			}
			b, _ := json.Marshal(p)
			params = string(b)
		}
		return &waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
			Body: &waE2E.InteractiveResponseMessage_Body{Text: proto.String(opt.Label), Format: waE2E.InteractiveResponseMessage_Body_DEFAULT.Enum()},
			InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
				NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
					Name: proto.String(name), ParamsJSON: proto.String(params), Version: proto.Int32(3),
				},
			},
			ContextInfo: ctxInfo,
		}}, nil
	}
	return nil, fmt.Errorf("unsupported response kind %q", cmp.Or(ov.ResponseKind, opt.Kind))
}
