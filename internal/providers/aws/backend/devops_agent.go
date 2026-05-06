package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/devopsagent"
	devopsagenttypes "github.com/aws/aws-sdk-go-v2/service/devopsagent/types"
)

// AgentSpace is a summary of an AWS DevOps Agent agent space.
type AgentSpace struct {
	AgentSpaceId string
	Name         string
	Desc         string
}

// Title returns the name for list display.
func (a AgentSpace) Title() string { return a.Name }

// Description returns the ID and description for list display.
func (a AgentSpace) Description() string {
	if a.Desc != "" {
		return a.Desc
	}
	return a.AgentSpaceId
}

// FilterValue returns text used for list filtering.
func (a AgentSpace) FilterValue() string { return a.Name + " " + a.AgentSpaceId }

// DevOpsAgentChatResult holds the response from a single SendMessage round-trip.
type DevOpsAgentChatResult struct {
	// Response is the assistant's assembled text reply.
	Response string
	// ExecutionId is the chat session identifier; returned on the first turn and
	// must be passed back on every subsequent SendMessage call.
	ExecutionId string
}

type devopsAgentAPI interface {
	ListAgentSpaces(
		ctx context.Context,
		params *devopsagent.ListAgentSpacesInput,
		optFns ...func(*devopsagent.Options),
	) (*devopsagent.ListAgentSpacesOutput, error)

	CreateChat(
		ctx context.Context,
		params *devopsagent.CreateChatInput,
		optFns ...func(*devopsagent.Options),
	) (*devopsagent.CreateChatOutput, error)

	SendMessage(
		ctx context.Context,
		params *devopsagent.SendMessageInput,
		optFns ...func(*devopsagent.Options),
	) (*devopsagent.SendMessageOutput, error)
}

// DevOpsAgentService wraps the AWS DevOps Agent client.
type DevOpsAgentService struct {
	api devopsAgentAPI
}

// InitDevOpsAgentService creates a DevOpsAgentService from an AWS config.
func InitDevOpsAgentService(cfg aws.Config) *DevOpsAgentService {
	slog.Debug("Initializing DevOps Agent Service")
	return &DevOpsAgentService{api: devopsagent.NewFromConfig(cfg)}
}

// ListAgentSpaces returns all agent spaces visible to the caller, collecting
// all pages.
func (s *DevOpsAgentService) ListAgentSpaces(ctx context.Context) ([]AgentSpace, error) {
	var spaces []AgentSpace
	var nextToken *string

	for {
		out, err := s.api.ListAgentSpaces(ctx, &devopsagent.ListAgentSpacesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list agent spaces: %w", err)
		}

		for _, sp := range out.AgentSpaces {
			if sp.AgentSpaceId == nil {
				continue
			}
			space := AgentSpace{AgentSpaceId: *sp.AgentSpaceId}
			if sp.Name != nil {
				space.Name = *sp.Name
			}
			if sp.Description != nil {
				space.Desc = *sp.Description
			}
			spaces = append(spaces, space)
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	slog.Debug(fmt.Sprintf("devops agent: found %d agent spaces", len(spaces)))
	return spaces, nil
}

// Chat sends a message to the given agent space.
//
// If executionId is empty, a new chat session is created first via CreateChat
// and the resulting ID is returned in DevOpsAgentChatResult.ExecutionId.
// On subsequent turns pass the returned ExecutionId back as executionId.
func (s *DevOpsAgentService) Chat(
	ctx context.Context,
	agentSpaceId string,
	executionId string,
	message string,
) (*DevOpsAgentChatResult, error) {
	// Create a new chat session on the first turn.
	if executionId == "" {
		chatOut, err := s.api.CreateChat(ctx, &devopsagent.CreateChatInput{
			AgentSpaceId: aws.String(agentSpaceId),
			UserType:     devopsagenttypes.UserTypeIam,
		})
		if err != nil {
			return nil, fmt.Errorf("create chat: %w", err)
		}
		executionId = *chatOut.ExecutionId
	}

	msgOut, err := s.api.SendMessage(ctx, &devopsagent.SendMessageInput{
		AgentSpaceId: aws.String(agentSpaceId),
		ExecutionId:  aws.String(executionId),
		Content:      aws.String(message),
	})
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	defer msgOut.GetStream().Close()

	// Consume the event stream.
	//
	// The DevOps Agent service sends text in two ways:
	//   • ContentBlockDelta / TextDelta — incremental text chunks (primary path)
	//   • ContentBlockStop.Text        — complete block text (may be nil)
	//   • Summary.Content              — a summary of agent actions taken
	//
	// We collect all text from deltas per block, then commit each block on
	// ContentBlockStop (preferring the pre-assembled Text when provided).
	// Summary events are appended after all content blocks.
	var (
		response strings.Builder
		blockBuf strings.Builder
		summary  strings.Builder
	)

	for event := range msgOut.GetStream().Events() {
		switch e := event.(type) {

		case *devopsagenttypes.SendMessageEventsMemberContentBlockStart:
			blockBuf.Reset()

		case *devopsagenttypes.SendMessageEventsMemberContentBlockDelta:
			if textDelta, ok := e.Value.Delta.(*devopsagenttypes.SendMessageContentBlockDeltaMemberTextDelta); ok {
				if textDelta.Value.Text != nil {
					blockBuf.WriteString(*textDelta.Value.Text)
				}
			}

		case *devopsagenttypes.SendMessageEventsMemberContentBlockStop:
			// Prefer the pre-assembled Text field when the service provides it;
			// otherwise fall back to the text we accumulated from deltas.
			blockText := blockBuf.String()
			if e.Value.Text != nil && *e.Value.Text != "" {
				blockText = *e.Value.Text
			}
			if blockText != "" {
				if response.Len() > 0 {
					response.WriteString("\n")
				}
				response.WriteString(blockText)
			}
			blockBuf.Reset()

		case *devopsagenttypes.SendMessageEventsMemberSummary:
			if e.Value.Content != nil && *e.Value.Content != "" {
				summary.WriteString(*e.Value.Content)
			}

		case *devopsagenttypes.SendMessageEventsMemberResponseFailed:
			errCode := "unknown"
			errMsg := "response failed"
			if e.Value.ErrorCode != nil {
				errCode = *e.Value.ErrorCode
			}
			if e.Value.ErrorMessage != nil {
				errMsg = *e.Value.ErrorMessage
			}
			return nil, fmt.Errorf("%s: %s", errCode, errMsg)
		}
	}

	if err := msgOut.GetStream().Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	// Combine content blocks and summary into the final response.
	// If content blocks are empty but a summary exists, use the summary alone.
	var final strings.Builder
	if response.Len() > 0 {
		final.WriteString(response.String())
	}
	if summary.Len() > 0 {
		if final.Len() > 0 {
			final.WriteString("\n\n")
		}
		final.WriteString(summary.String())
	}

	return &DevOpsAgentChatResult{
		Response:    final.String(),
		ExecutionId: executionId,
	}, nil
}
