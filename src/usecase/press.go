package usecase

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
)

func (service serviceSend) GetInteractive(ctx context.Context, request domainSend.InteractiveRequest) (response domainSend.InteractiveResponse, err error) {
	if request.MessageID == "" {
		return response, pkgError.ValidationError("message_id is required")
	}
	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}
	rec, msg, err := whatsapp.LoadInteractivePrompt(client, request.MessageID)
	if err != nil {
		return response, pkgError.ValidationError(err.Error())
	}
	response.MessageID = rec.MessageID
	response.ChatJID = rec.ChatJID
	response.SenderJID = rec.SenderJID
	response.ReceivedAt = rec.ReceivedAt.UTC().Format(time.RFC3339)
	response.Prompt = utils.ParseInteractivePrompt(msg)
	return response, nil
}

func (service serviceSend) SendPress(ctx context.Context, request domainSend.PressRequest) (response domainSend.PressResponse, err error) {
	if request.MessageID == "" {
		return response, pkgError.ValidationError("message_id is required")
	}
	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return response, pkgError.ErrWaCLI
	}
	rec, msg, err := whatsapp.LoadInteractivePrompt(client, request.MessageID)
	if err != nil {
		return response, pkgError.ValidationError(err.Error())
	}
	prompt := utils.ParseInteractivePrompt(msg)
	if prompt == nil {
		return response, pkgError.ValidationError("stored message is not an interactive prompt")
	}
	opt, err := utils.SelectInteractiveOption(prompt.Options, request.OptionID, request.Index, request.Label)
	if err != nil {
		return response, pkgError.ValidationError(err.Error())
	}
	chat, participant, err := whatsapp.ReplyAddress(ctx, client, rec)
	if err != nil {
		return response, err
	}
	if request.Phone != "" {
		forced, verr := utils.ValidateJidWithLogin(client, request.Phone)
		if verr != nil {
			return response, verr
		}
		if verr = whatsapp.CheckPhoneOverride(chat, forced); verr != nil {
			return response, pkgError.ValidationError(verr.Error())
		}
		chat = forced
		if participant == rec.SenderJID && strings.HasSuffix(participant, "@lid") {
			participant = forced.String()
		}
	}
	reply, err := utils.BuildInteractiveResponse(msg, opt, rec.MessageID, participant, utils.PressOverrides{
		ResponseKind: request.ResponseKind, FlowName: request.FlowName, ParamsJSON: request.ParamsJSON,
	})
	if err != nil {
		return response, pkgError.ValidationError(err.Error())
	}
	ts, err := service.wrapSendMessage(ctx, client, chat, reply, opt.Label)
	if err != nil {
		return response, err
	}
	response.MessageID = ts.ID
	response.Pressed = opt
	response.ResponseKind = cmp.Or(request.ResponseKind, opt.Kind)
	response.SentTo = chat.String()
	response.Status = fmt.Sprintf("Pressed option %d (%q) on %s (server timestamp: %s)", opt.Index, opt.Label, rec.MessageID, ts.Timestamp.String())
	return response, nil
}
