package provider

import (
	"context"
	"errors"
	"time"

	"non24.app/server/internal/chatgpt"
)

// ChatGPT is the provider name for the owner's ChatGPT plan (ADR-0066).
const ChatGPT Name = "chatgpt"

// chatGPTTimeout bounds one answer. Models a ChatGPT plan offers can think
// for a while before they answer.
const chatGPTTimeout = 60 * time.Second

// chatGPTClient asks the model the owner chose, through their ChatGPT plan.
type chatGPTClient struct {
	account *chatgpt.Account
}

// NewChatGPT is the provider for an account the owner signs in to from a
// device. It is configured while the account is connected with a model.
func NewChatGPT(account *chatgpt.Account) LLM {
	return chatGPTClient{account: account}
}

func (c chatGPTClient) Complete(ctx context.Context, input Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, chatGPTTimeout)
	defer cancel()
	text, err := c.account.Respond(ctx, input.System, prompt(input))
	var failed *chatgpt.Error
	if errors.As(err, &failed) && failed.Problem == chatgpt.TooLarge {
		// As with any provider, the assistant asks once more with less.
		return Response{}, ErrContextTooLarge
	}
	if errors.As(err, &failed) {
		return Response{}, Unavailable{Reason: chatGPTReason(failed.Problem)}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Response{}, Unavailable{Reason: ReasonBusy}
	}
	if err != nil {
		return Response{}, err
	}
	return Response{Text: text}, nil
}

func (c chatGPTClient) Status() Status {
	status := c.account.Status()
	result := Status{Provider: string(ChatGPT), Model: status.Model, Auth: "chatgpt_plan"}
	switch {
	case status.State == chatgpt.Ended:
		result.Problem = ReasonSignInEnded
	case status.State != chatgpt.Connected:
		result.Problem = ReasonSignInNeeded
	case status.Model == "":
		result.Problem = ReasonModelNeeded
	default:
		result.Configured = true
	}
	return result
}

// chatGPTReason says what a ChatGPT problem asks of the owner.
func chatGPTReason(problem chatgpt.Problem) Reason {
	switch problem {
	case chatgpt.UsageLimit:
		return ReasonUsageLimit
	case chatgpt.SignInEnded, chatgpt.AccountMismatch:
		return ReasonSignInEnded
	case chatgpt.PlanNotShared:
		return ReasonPlanNotShared
	case chatgpt.NotEligible:
		return ReasonNotEligible
	case chatgpt.NotEnabled:
		return ReasonNotEnabled
	case chatgpt.ModelUnavailable:
		return ReasonModelUnavailable
	case chatgpt.Refused:
		return ReasonRefused
	default:
		return ReasonBusy
	}
}
