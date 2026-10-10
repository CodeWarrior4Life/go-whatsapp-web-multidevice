package whatsapp

import (
	"context"
	"testing"
)

// A 1:1 prompt from a LID must be answered at the sender's phone number:
// the server rejects sends to an unmapped @lid with error 479.
func TestReplyAddressSwapsLIDForStoredPhone(t *testing.T) {
	rec := &InteractiveRecord{
		MessageID: "FE37", ChatJID: "192930568519785@lid", SenderJID: "192930568519785@lid",
		SenderAltJID: "573167668245@s.whatsapp.net",
	}
	chat, participant, err := ReplyAddress(context.Background(), nil, rec)
	if err != nil {
		t.Fatal(err)
	}
	if chat.String() != "573167668245@s.whatsapp.net" || participant != "573167668245@s.whatsapp.net" {
		t.Fatalf("got chat %s participant %s", chat, participant)
	}
}

func TestReplyAddressKeepsPhoneChats(t *testing.T) {
	rec := &InteractiveRecord{MessageID: "A1", ChatJID: "15551234567@s.whatsapp.net", SenderJID: "15551234567@s.whatsapp.net"}
	chat, participant, err := ReplyAddress(context.Background(), nil, rec)
	if err != nil || chat.String() != rec.ChatJID || participant != rec.SenderJID {
		t.Fatalf("got %s %s %v", chat, participant, err)
	}
}

func TestReplyAddressUnmappedLIDStaysLID(t *testing.T) {
	rec := &InteractiveRecord{MessageID: "B1", ChatJID: "111@lid", SenderJID: "111@lid"}
	chat, _, err := ReplyAddress(context.Background(), nil, rec)
	if err != nil || chat.String() != "111@lid" {
		t.Fatalf("got %s %v", chat, err)
	}
}
