package send

import "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"

// InteractiveRequest reads the tappable options of a stored inbound prompt.
type InteractiveRequest struct {
	MessageID string `json:"message_id" form:"message_id"`
}

type InteractiveResponse struct {
	MessageID  string                   `json:"message_id"`
	ChatJID    string                   `json:"chat_jid"`
	SenderJID  string                   `json:"sender_jid"`
	ReceivedAt string                   `json:"received_at"`
	Prompt     *utils.InteractivePrompt `json:"prompt"`
}

// PressRequest taps one option of a stored inbound prompt. Pick the option by
// OptionID, Index (1-based) or Label; the first one given wins.
type PressRequest struct {
	MessageID string `json:"message_id" form:"message_id"`
	OptionID  string `json:"option_id,omitempty" form:"option_id"`
	Index     int    `json:"index,omitempty" form:"index"`
	Label     string `json:"label,omitempty" form:"label"`
	// Advanced: force the response variant when a bot rejects the derived one.
	ResponseKind string `json:"response_kind,omitempty" form:"response_kind"`
	FlowName     string `json:"flow_name,omitempty" form:"flow_name"`
	ParamsJSON   string `json:"params_json,omitempty" form:"params_json"`
}

type PressResponse struct {
	MessageID    string                  `json:"message_id"`
	Status       string                  `json:"status"`
	Pressed      utils.InteractiveOption `json:"pressed"`
	ResponseKind string                  `json:"response_kind"`
}
