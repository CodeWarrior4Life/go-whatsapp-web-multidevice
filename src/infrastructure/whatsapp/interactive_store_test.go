package whatsapp

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
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

func TestCheckPhoneOverride(t *testing.T) {
	lid := types.NewJID("192930568519785", types.HiddenUserServer)
	pn := types.NewJID("573167668245", types.DefaultUserServer)
	other := types.NewJID("15550001111", types.DefaultUserServer)
	if err := CheckPhoneOverride(lid, pn); err != nil {
		t.Fatalf("unresolved LID must allow an override: %v", err)
	}
	if err := CheckPhoneOverride(pn, pn); err != nil {
		t.Fatalf("override equal to the resolved number must pass: %v", err)
	}
	if err := CheckPhoneOverride(pn, other); err == nil {
		t.Fatal("override to a different number on a resolved chat must be refused")
	}
}
