package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func listPromptMsg() *waE2E.Message {
	return &waE2E.Message{ListMessage: &waE2E.ListMessage{
		Title: proto.String("Cedimed"), Description: proto.String("Elige una opcion"), ButtonText: proto.String("Ver opciones"),
		ListType: waE2E.ListMessage_SINGLE_SELECT.Enum(),
		Sections: []*waE2E.ListMessage_Section{{
			Title: proto.String("Servicios"),
			Rows: []*waE2E.ListMessage_Row{
				{RowID: proto.String("cita"), Title: proto.String("Solicitar una cita"), Description: proto.String("Agenda tu examen")},
				{RowID: proto.String("asesor"), Title: proto.String("Hablar con un asesor")},
			},
		}},
	}}
}

func buttonsPromptMsg() *waE2E.Message {
	return &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{
		ContentText: proto.String("Please choose"),
		Buttons: []*waE2E.ButtonsMessage_Button{
			{ButtonID: proto.String("sales"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Sales")}},
			{ButtonID: proto.String("support"), ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Support")}},
		},
	}}
}

func nativeFlowPromptMsg() *waE2E.Message {
	return &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
		Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Hola, en que te ayudamos?")},
		InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
			Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
				{Name: proto.String("quick_reply"), ButtonParamsJSON: proto.String(`{"display_text":"Solicitar una cita","id":"btn_cita"}`)},
				{Name: proto.String("cta_url"), ButtonParamsJSON: proto.String(`{"display_text":"Web","url":"https://example.org"}`)},
				{Name: proto.String("single_select"), ButtonParamsJSON: proto.String(`{"title":"Menu","sections":[{"rows":[{"id":"r1","title":"Resonancia","description":"MRI"}]}]}`)},
			},
		}},
	}}
}

func templatePromptMsg() *waE2E.Message {
	return &waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
		HydratedContentText: proto.String("Confirm your appointment"),
		HydratedButtons: []*waE2E.HydratedTemplateButton{
			{Index: proto.Uint32(0), HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{QuickReplyButton: &waE2E.HydratedTemplateButton_HydratedQuickReplyButton{DisplayText: proto.String("Yes"), ID: proto.String("yes")}}},
			{Index: proto.Uint32(1), HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{QuickReplyButton: &waE2E.HydratedTemplateButton_HydratedQuickReplyButton{DisplayText: proto.String("No"), ID: proto.String("no")}}},
		},
	}}}
}

func TestParseListPrompt(t *testing.T) {
	p := ParseInteractivePrompt(listPromptMsg())
	if p == nil || p.Kind != InteractiveKindList || len(p.Options) != 2 {
		t.Fatalf("unexpected prompt %#v", p)
	}
	o := p.Options[0]
	if o.Index != 1 || o.ID != "cita" || o.Label != "Solicitar una cita" || o.Section != "Servicios" || !o.Pressable {
		t.Fatalf("unexpected option %#v", o)
	}
	if p.Button != "Ver opciones" {
		t.Fatalf("button text %q", p.Button)
	}
}

func TestPressListBuildsListResponseQuotingPrompt(t *testing.T) {
	prompt := listPromptMsg()
	p := ParseInteractivePrompt(prompt)
	opt, err := SelectInteractiveOption(p.Options, "", 0, "solicitar una CITA")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := BuildInteractiveResponse(prompt, opt, "MSG1", "573167668245@s.whatsapp.net", PressOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	r := reply.GetListResponseMessage()
	if r.GetSingleSelectReply().GetSelectedRowID() != "cita" || r.GetTitle() != "Solicitar una cita" || r.GetListType() != waE2E.ListResponseMessage_SINGLE_SELECT {
		t.Fatalf("bad list response %v", r)
	}
	ci := r.GetContextInfo()
	if ci.GetStanzaID() != "MSG1" || ci.GetParticipant() != "573167668245@s.whatsapp.net" || ci.GetQuotedMessage().GetListMessage() == nil {
		t.Fatalf("bad context info %v", ci)
	}
	// Round-trips through the existing inbound parser as the same selection.
	sel := BuildSelectionPayload(reply)
	if sel == nil || sel.Kind != "list" || sel.SelectedID != "cita" {
		t.Fatalf("selection %#v", sel)
	}
}

func TestPressButtons(t *testing.T) {
	prompt := buttonsPromptMsg()
	p := ParseInteractivePrompt(prompt)
	opt, err := SelectInteractiveOption(p.Options, "support", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := BuildInteractiveResponse(prompt, opt, "B1", "", PressOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	r := reply.GetButtonsResponseMessage()
	if r.GetSelectedButtonID() != "support" || r.GetSelectedDisplayText() != "Support" || r.GetType() != waE2E.ButtonsResponseMessage_DISPLAY_TEXT {
		t.Fatalf("bad buttons response %v", r)
	}
	if r.GetContextInfo().Participant != nil {
		t.Fatalf("empty participant must stay unset")
	}
}

func TestPressTemplateQuickReply(t *testing.T) {
	prompt := templatePromptMsg()
	p := ParseInteractivePrompt(prompt)
	opt, err := SelectInteractiveOption(p.Options, "", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := BuildInteractiveResponse(prompt, opt, "T1", "", PressOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	r := reply.GetTemplateButtonReplyMessage()
	if r.GetSelectedID() != "no" || r.GetSelectedDisplayText() != "No" || r.GetSelectedIndex() != 1 {
		t.Fatalf("bad template reply %v", r)
	}
}

func TestNativeFlowOptionsAndPress(t *testing.T) {
	prompt := nativeFlowPromptMsg()
	p := ParseInteractivePrompt(prompt)
	if p == nil || len(p.Options) != 3 {
		t.Fatalf("unexpected prompt %#v", p)
	}
	if p.Options[1].Pressable || p.Options[1].URL != "https://example.org" {
		t.Fatalf("cta_url must be listed but not pressable: %#v", p.Options[1])
	}
	if _, err := BuildInteractiveResponse(prompt, p.Options[1], "N1", "", PressOverrides{}); err == nil {
		t.Fatal("pressing a url button must fail")
	}

	reply, err := BuildInteractiveResponse(prompt, p.Options[0], "N1", "", PressOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	nf := reply.GetInteractiveResponseMessage().GetNativeFlowResponseMessage()
	if nf.GetName() != "quick_reply" || nf.GetParamsJSON() != `{"id":"btn_cita"}` || nf.GetVersion() != 3 {
		t.Fatalf("bad native flow response %v", nf)
	}
	if reply.GetInteractiveResponseMessage().GetBody().GetText() != "Solicitar una cita" {
		t.Fatal("body text should carry the label")
	}
	if sel := BuildSelectionPayload(reply); sel == nil || sel.SelectedID != "btn_cita" {
		t.Fatalf("selection %#v", sel)
	}

	row := p.Options[2]
	if row.ID != "r1" || row.FlowName != "single_select" || row.Section != "Menu" {
		t.Fatalf("single_select row %#v", row)
	}
	reply, err = BuildInteractiveResponse(prompt, row, "N1", "", PressOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]string
	_ = json.Unmarshal([]byte(reply.GetInteractiveResponseMessage().GetNativeFlowResponseMessage().GetParamsJSON()), &params)
	if params["id"] != "r1" || params["description"] != "MRI" {
		t.Fatalf("params %v", params)
	}
}

func TestPressOverrides(t *testing.T) {
	prompt := nativeFlowPromptMsg()
	p := ParseInteractivePrompt(prompt)
	reply, err := BuildInteractiveResponse(prompt, p.Options[2], "N1", "", PressOverrides{FlowName: "menu_options", ParamsJSON: `{"id":"r1"}`})
	if err != nil {
		t.Fatal(err)
	}
	nf := reply.GetInteractiveResponseMessage().GetNativeFlowResponseMessage()
	if nf.GetName() != "menu_options" || nf.GetParamsJSON() != `{"id":"r1"}` {
		t.Fatalf("override ignored %v", nf)
	}
	reply, err = BuildInteractiveResponse(prompt, p.Options[2], "N1", "", PressOverrides{ResponseKind: InteractiveKindList})
	if err != nil || reply.GetListResponseMessage().GetSingleSelectReply().GetSelectedRowID() != "r1" {
		t.Fatalf("response kind override failed: %v %v", reply, err)
	}
}

func TestUnwrapViewOnceAndTemplateWrappedInteractive(t *testing.T) {
	wrapped := &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
		TemplateMessage: &waE2E.TemplateMessage{Format: &waE2E.TemplateMessage_InteractiveMessageTemplate{
			InteractiveMessageTemplate: nativeFlowPromptMsg().GetInteractiveMessage(),
		}},
	}}}
	if !IsInteractivePrompt(wrapped) {
		t.Fatal("wrapped template interactive should be a prompt")
	}
	p := ParseInteractivePrompt(wrapped)
	if p.Kind != InteractiveKindNativeFlow || p.Options[0].ID != "btn_cita" {
		t.Fatalf("unexpected %#v", p)
	}
}

func TestSelectErrorsAndNonPrompts(t *testing.T) {
	p := ParseInteractivePrompt(listPromptMsg())
	for _, c := range []struct {
		id    string
		index int
		label string
	}{{"nope", 0, ""}, {"", 9, ""}, {"", 0, "nope"}, {"", 0, ""}} {
		if _, err := SelectInteractiveOption(p.Options, c.id, c.index, c.label); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found for %+v, got %v", c, err)
		}
	}
	if IsInteractivePrompt(&waE2E.Message{Conversation: proto.String("hola")}) {
		t.Fatal("plain text is not a prompt")
	}
	if IsInteractivePrompt(nil) {
		t.Fatal("nil is not a prompt")
	}
}
