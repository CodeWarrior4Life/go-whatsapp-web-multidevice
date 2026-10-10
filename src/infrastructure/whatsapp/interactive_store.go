package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Chat storage keeps only the rendered text of a business prompt, which loses
// the option ids a tap must send back. The raw prompt proto is kept here, one
// small file per message, so /message/:id/interactive and /message/:id/press
// work after a restart too.

const interactivePromptTTL = 14 * 24 * time.Hour

var safeFileName = regexp.MustCompile(`[^A-Za-z0-9._@-]`)

// InteractiveRecord is a stored inbound interactive prompt.
type InteractiveRecord struct {
	MessageID string `json:"message_id"`
	ChatJID   string `json:"chat_jid"`
	SenderJID string `json:"sender_jid"`
	// Phone-number (@s.whatsapp.net) form of the sender when it wrote from a
	// LID; sends addressed to an unmapped @lid fail with server error 479.
	SenderAltJID string    `json:"sender_alt_jid,omitempty"`
	ReceivedAt   time.Time `json:"received_at"`
	Proto        []byte    `json:"proto"`
}

func interactiveDir(client *whatsmeow.Client) (string, error) {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return "", fmt.Errorf("whatsapp client is not logged in")
	}
	owner := safeFileName.ReplaceAllString(client.Store.ID.ToNonAD().String(), "_")
	return filepath.Join(config.PathStorages, "interactive", owner), nil
}

// SaveInteractivePrompt stores evt if it carries a tappable prompt. Errors are
// logged, never returned: this must not disturb normal message handling.
func SaveInteractivePrompt(client *whatsmeow.Client, evt *events.Message) {
	if evt == nil || evt.Info.IsFromMe || !utils.IsInteractivePrompt(evt.Message) {
		return
	}
	dir, err := interactiveDir(client)
	if err != nil {
		return
	}
	raw, err := proto.Marshal(evt.Message)
	if err != nil {
		logrus.Warnf("interactive: marshal %s: %v", evt.Info.ID, err)
		return
	}
	rec := InteractiveRecord{
		MessageID:  evt.Info.ID,
		ChatJID:    evt.Info.Chat.String(),
		SenderJID:  evt.Info.Sender.ToNonAD().String(),
		ReceivedAt: evt.Info.Timestamp,
		Proto:      raw,
	}
	if !evt.Info.SenderAlt.IsEmpty() {
		rec.SenderAltJID = evt.Info.SenderAlt.ToNonAD().String()
	}
	body, _ := json.Marshal(rec)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logrus.Warnf("interactive: mkdir: %v", err)
		return
	}
	path := filepath.Join(dir, safeFileName.ReplaceAllString(evt.Info.ID, "_")+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		logrus.Warnf("interactive: write %s: %v", evt.Info.ID, err)
		return
	}
	logrus.Infof("interactive: stored prompt %s from %s", evt.Info.ID, rec.SenderJID)
	pruneInteractive(dir)
}

// ReplyAddress returns where a press reply must go and who to quote: the
// stored chat, with a LID swapped for its phone-number form (the stored alt,
// else the client's LID map). A 1:1 chat with an unmapped @lid is rejected by
// the server with error 479.
func ReplyAddress(ctx context.Context, client *whatsmeow.Client, rec *InteractiveRecord) (chat types.JID, participant string, err error) {
	chat, err = types.ParseJID(rec.ChatJID)
	if err != nil {
		return chat, "", fmt.Errorf("stored chat jid %q: %w", rec.ChatJID, err)
	}
	participant = rec.SenderJID
	if chat.Server == types.HiddenUserServer {
		if rec.SenderAltJID != "" && rec.ChatJID == rec.SenderJID {
			if alt, perr := types.ParseJID(rec.SenderAltJID); perr == nil {
				chat = alt
			}
		}
		chat = NormalizeJIDFromLID(ctx, chat, client)
	}
	if sender, perr := types.ParseJID(rec.SenderJID); perr == nil && sender.Server == types.HiddenUserServer {
		if rec.SenderAltJID != "" {
			participant = rec.SenderAltJID
		} else {
			participant = NormalizeJIDFromLID(ctx, sender, client).String()
		}
	}
	return chat, participant, nil
}

// LoadInteractivePrompt returns the stored prompt for messageID.
func LoadInteractivePrompt(client *whatsmeow.Client, messageID string) (*InteractiveRecord, *waE2E.Message, error) {
	dir, err := interactiveDir(client)
	if err != nil {
		return nil, nil, err
	}
	body, err := os.ReadFile(filepath.Join(dir, safeFileName.ReplaceAllString(messageID, "_")+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no interactive prompt stored for message %s (only prompts received since the press feature was deployed are kept)", messageID)
		}
		return nil, nil, err
	}
	var rec InteractiveRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, nil, err
	}
	var msg waE2E.Message
	if err := proto.Unmarshal(rec.Proto, &msg); err != nil {
		return nil, nil, err
	}
	return &rec, &msg, nil
}

func pruneInteractive(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-interactivePromptTTL)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
