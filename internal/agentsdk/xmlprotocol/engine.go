package xmlprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
)

const (
	DefaultMaxSteps = 200
	maxMaxStepsCap  = 2000
)

type FailureRecorder func(toolName, toolCallID, args string, err error)

type StepRecord struct {
	VisibleContent    string
	AssistantContent  string
	ToolCalls         []agentsdk.ToolCall
	ToolResults       []ToolResult
	ToolResultMessage string
}

type StepObserver func(StepRecord)

type ToolResult struct {
	ToolName   string
	ToolCallID string
	OK         bool
	OutputJSON string
	Error      string
}

type Callbacks struct {
	OnContent agentsdk.StreamCallback
	OnTrace   func(string)
	OnError   func(string)

	RecordFailure FailureRecorder

	ObserveStep  StepObserver
	ObserveFinal func(visibleContent, assistantContent string)
	OnStepStart  func(step int)

	// EventSink receives structured, append-only events for observability and replay.
	// It is called synchronously; implementations should offload persistence if needed.
	EventSink agentsdk.EventSink
}

type RunLoopInput struct {
	Client   agentsdk.Client
	Messages []agentsdk.Message

	// LLMOptions are forwarded verbatim to Client.ChatCompletionStream.
	LLMOptions *agentsdk.ChatCompletionOptions

	// ContextCompressThresholdRunes enables best-effort in-loop context compression.
	// When > 0 and the rolling conversation exceeds this approximate rune budget,
	// the loop summarizes older messages into a short background note.
	//
	// This is especially helpful for long tool loops where tool_result payloads
	// bloat the prompt and degrade tool-call accuracy.
	ContextCompressThresholdRunes int

	Executor agentsdk.ToolExecutor

	// MaxSteps is the upper bound of tool-loop iterations.
	// When <= 0, DefaultMaxSteps is used.
	MaxSteps int

	Callbacks Callbacks
}

func RunLoop(ctx context.Context, in RunLoopInput) (string, error) {
	if in.Client == nil {
		err := errors.New("missing llm client")
		emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, 0, agentsdk.ErrorEvent{Error: err.Error()})
		return "", err
	}
	if in.Executor == nil {
		err := errors.New("missing tool executor")
		emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, 0, agentsdk.ErrorEvent{Error: err.Error()})
		return "", err
	}

	msgs := make([]agentsdk.Message, 0, len(in.Messages)+8)
	msgs = append(msgs, in.Messages...)

	var combined strings.Builder

	maxSteps := normalizeMaxSteps(in.MaxSteps)
	for step := 0; step < maxSteps; step++ {
		toolStep := step
		if toolStep < 0 {
			toolStep = 0
		}
		if in.Callbacks.OnStepStart != nil {
			in.Callbacks.OnStepStart(step)
		}

		if step > 0 && in.ContextCompressThresholdRunes > 0 {
			before := len(msgs)
			compressedMsgs, didCompress, compErr := compressConversationIfNeeded(
				ctx,
				in.Client,
				msgs,
				in.ContextCompressThresholdRunes,
				in.LLMOptions,
				in.Callbacks.EventSink,
				toolStep,
			)
			if compErr != nil {
				emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, toolStep, agentsdk.ErrorEvent{Error: compErr.Error()})
				return combined.String(), compErr
			}
			if didCompress {
				msgs = compressedMsgs
				if in.LLMOptions != nil {
					in.LLMOptions.CacheEpoch++
				}
				trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Context compressed: messages=%d -> %d", before, len(msgs)))
			}
		}

		var raw strings.Builder
		var visible strings.Builder
		filter := &StreamFilter{}

		type streamGate struct {
			decided bool
			blocked bool
			buf     strings.Builder
		}

		var gate streamGate
		resetGate := func() {
			gate.decided = false
			gate.blocked = false
			gate.buf.Reset()
		}
		resetGate()

		emitContent := func(delta string) error {
			if delta == "" || gate.blocked {
				return nil
			}

			if !gate.decided {
				gate.buf.WriteString(delta)

				probe := strings.TrimLeftFunc(gate.buf.String(), func(r rune) bool {
					return r == ' ' || r == '\t' || r == '\n' || r == '\r'
				})
				if probe == "" {
					return nil
				}

				lower := strings.ToLower(probe)
				if strings.HasPrefix(probe, "{") || strings.HasPrefix(probe, "[") {
					gate.blocked = true
					gate.decided = true
					gate.buf.Reset()
					return nil
				}

				const jsonFence = "```json"
				if strings.HasPrefix(jsonFence, lower) && len(lower) < len(jsonFence) {
					return nil
				}
				if strings.HasPrefix(lower, jsonFence) {
					gate.blocked = true
					gate.decided = true
					gate.buf.Reset()
					return nil
				}

				gate.decided = true
				buffered := gate.buf.String()
				gate.buf.Reset()
				delta = buffered
			}

			visible.WriteString(delta)
			if in.Callbacks.OnContent != nil {
				return in.Callbacks.OnContent(delta)
			}
			return nil
		}

		emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindLLMRequest, toolStep, agentsdk.LLMRequestEvent{
			Messages: append([]agentsdk.Message(nil), msgs...),
			Options:  cloneOptions(in.LLMOptions),
		})

		err := in.Client.ChatCompletionStream(ctx, msgs, in.LLMOptions, func(chunk string) error {
			raw.WriteString(chunk)
			emitted := filter.Feed(chunk)
			if emitted == "" {
				return nil
			}
			return emitContent(emitted)
		})
		if tail := filter.Flush(); tail != "" {
			if cbErr := emitContent(tail); cbErr != nil && err == nil {
				err = cbErr
			}
		}

		stepVisible := visible.String()
		if err != nil && stepVisible != "" {
			combined.WriteString(stepVisible)
		}

		if err != nil {
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindLLMResponse, toolStep, agentsdk.LLMResponseEvent{
				Raw:     raw.String(),
				Visible: stepVisible,
				Error:   err.Error(),
			})
		} else {
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindLLMResponse, toolStep, agentsdk.LLMResponseEvent{
				Raw:     raw.String(),
				Visible: stepVisible,
				Error:   "",
			})
		}
		if err != nil {
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, toolStep, agentsdk.ErrorEvent{Error: err.Error()})
			return combined.String(), err
		}

		cleanedRaw := StripThinking(raw.String())
		assistantForHistory := strings.TrimSpace(cleanedRaw)
		if assistantForHistory == "" {
			assistantForHistory = stepVisible
		}

		toolBlock, ok, sawToolStart := extractLatestToolDataFromCleaned(cleanedRaw)
		if !ok {
			if isForbiddenJSONToolCall(cleanedRaw) {
				trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Tool protocol error: %v", errJSONToolCallForbidden))
				emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, toolStep, agentsdk.ErrorEvent{Error: errJSONToolCallForbidden.Error()})

				toolName := "tool_protocol"
				toolCallID := fmt.Sprintf("xml_%d_protocol", step)
				result := ToolResult{
					ToolName:   toolName,
					ToolCallID: toolCallID,
					OK:         false,
					Error:      errJSONToolCallForbidden.Error(),
					OutputJSON: fmt.Sprintf(`{"error":%q}`, errJSONToolCallForbidden.Error()),
				}
				emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, toolStep, agentsdk.ToolResultEvent{
					ToolName:   result.ToolName,
					ToolCallID: result.ToolCallID,
					OK:         result.OK,
					OutputJSON: result.OutputJSON,
					Error:      result.Error,
				})

				toolResultMsg := buildToolResultMessage([]ToolResult{result})
				if in.Callbacks.RecordFailure != nil {
					in.Callbacks.RecordFailure(toolName, toolCallID, assistantForHistory, errJSONToolCallForbidden)
				}
				if in.Callbacks.ObserveStep != nil {
					in.Callbacks.ObserveStep(StepRecord{
						VisibleContent:    stepVisible,
						AssistantContent:  assistantForHistory,
						ToolCalls:         []agentsdk.ToolCall{{ID: toolCallID, Name: toolName}},
						ToolResults:       []ToolResult{result},
						ToolResultMessage: toolResultMsg,
					})
				}

				msgs = append(msgs,
					agentsdk.Message{Role: "assistant", Content: assistantForHistory},
					agentsdk.Message{Role: "user", Content: toolResultMsg},
				)
				continue
			}

			if sawToolStart {
				if recovered, ok := recoverTruncatedWriteFileAppend(cleanedRaw); ok {
					combinedStep, continued, err := in.recoverTruncatedWriteFileAppend(ctx, step, recovered, stepVisible, &msgs)
					if continued {
						combined.WriteString(combinedStep)
						continue
					}
					if err != nil {
						return combined.String(), err
					}
				}

				protoErr := errors.New("truncated <tool_data> block")
				trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Tool protocol error: %v", protoErr))
				emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, toolStep, agentsdk.ErrorEvent{Error: protoErr.Error()})

				toolName := "tool_protocol"
				toolCallID := fmt.Sprintf("xml_%d_protocol", step)
				result := ToolResult{
					ToolName:   toolName,
					ToolCallID: toolCallID,
					OK:         false,
					Error:      protoErr.Error(),
					OutputJSON: fmt.Sprintf(`{"error":%q}`, protoErr.Error()),
				}
				emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, toolStep, agentsdk.ToolResultEvent{
					ToolName:   result.ToolName,
					ToolCallID: result.ToolCallID,
					OK:         result.OK,
					OutputJSON: result.OutputJSON,
					Error:      result.Error,
				})

				toolResultMsg := buildToolResultMessage([]ToolResult{result})
				if in.Callbacks.RecordFailure != nil {
					in.Callbacks.RecordFailure(toolName, toolCallID, assistantForHistory, protoErr)
				}
				if in.Callbacks.ObserveStep != nil {
					in.Callbacks.ObserveStep(StepRecord{
						VisibleContent:    stepVisible,
						AssistantContent:  assistantForHistory,
						ToolCalls:         []agentsdk.ToolCall{{ID: toolCallID, Name: toolName}},
						ToolResults:       []ToolResult{result},
						ToolResultMessage: toolResultMsg,
					})
				}

				msgs = append(msgs,
					agentsdk.Message{Role: "assistant", Content: assistantForHistory},
					agentsdk.Message{Role: "user", Content: toolResultMsg},
				)
				continue
			}

			if stepVisible != "" {
				combined.WriteString(stepVisible)
			}
			if in.Callbacks.ObserveFinal != nil {
				in.Callbacks.ObserveFinal(stepVisible, assistantForHistory)
			}
			return combined.String(), nil
		}

		if stepVisible != "" {
			combined.WriteString(stepVisible)
		}
		parseRes, err := ParseToolDataWithRepairs(toolBlock)
		if err != nil {
			trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Tool protocol parse error: %v", err))
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, toolStep, agentsdk.ErrorEvent{Error: err.Error()})

			toolName := "tool_protocol"
			toolCallID := fmt.Sprintf("xml_%d_protocol", step)
			result := ToolResult{
				ToolName:   toolName,
				ToolCallID: toolCallID,
				OK:         false,
				Error:      err.Error(),
				OutputJSON: fmt.Sprintf(`{"error":%q}`, err.Error()),
			}
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, toolStep, agentsdk.ToolResultEvent{
				ToolName:   result.ToolName,
				ToolCallID: result.ToolCallID,
				OK:         result.OK,
				OutputJSON: result.OutputJSON,
				Error:      result.Error,
			})

			toolResultMsg := buildToolResultMessage([]ToolResult{result})
			if in.Callbacks.RecordFailure != nil {
				in.Callbacks.RecordFailure(toolName, toolCallID, toolBlock, err)
			}
			if in.Callbacks.ObserveStep != nil {
				in.Callbacks.ObserveStep(StepRecord{
					VisibleContent:    stepVisible,
					AssistantContent:  assistantForHistory,
					ToolCalls:         []agentsdk.ToolCall{{ID: toolCallID, Name: toolName}},
					ToolResults:       []ToolResult{result},
					ToolResultMessage: toolResultMsg,
				})
			}

			msgs = append(msgs,
				agentsdk.Message{Role: "assistant", Content: assistantForHistory},
				agentsdk.Message{Role: "user", Content: toolResultMsg},
			)
			continue
		}

		if len(parseRes.Repairs) > 0 && strings.TrimSpace(parseRes.CanonicalToolData) != "" {
			repaired := strings.TrimSpace(replaceLast(cleanedRaw, toolBlock, parseRes.CanonicalToolData))
			if repaired != "" {
				assistantForHistory = repaired
			}
		}

		calls := parseRes.Calls
		msgs = append(msgs, agentsdk.Message{Role: "assistant", Content: assistantForHistory})

		results := make([]ToolResult, 0, len(calls))
		recordedCalls := make([]agentsdk.ToolCall, 0, len(calls))
		for idx, call := range calls {
			toolName := strings.TrimSpace(call.ToolName)
			toolCallID := fmt.Sprintf("xml_%d_%d", step, idx)
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolCall, toolStep, agentsdk.ToolCallEvent{
				ID:     toolCallID,
				Name:   toolName,
				Fields: cloneFields(call.Fields),
				Raw:    call.Raw,
			})
			recordedCalls = append(recordedCalls, agentsdk.ToolCall{
				ID:     toolCallID,
				Name:   toolName,
				Fields: call.Fields,
				Raw:    call.Raw,
			})

			trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Running tool: %s", toolName))

			payload, toolErr := in.Executor.Execute(ctx, agentsdk.ToolCall{
				ID:     toolCallID,
				Name:   toolName,
				Fields: call.Fields,
				Raw:    call.Raw,
			})
			if toolErr != nil {
				payload = map[string]string{"error": fmt.Sprintf("Tool execution failed: %v", toolErr)}
			}

			response, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				trace(ctx, in.Callbacks, toolStep, fmt.Sprintf("Tool %s response error: %v", toolName, marshalErr))
				if in.Callbacks.RecordFailure != nil {
					in.Callbacks.RecordFailure(toolName, toolCallID, call.Raw, marshalErr)
				}
				response = []byte(fmt.Sprintf(`{"error":"failed to marshal tool response: %v"}`, marshalErr))
			}

			errMsg := ""
			if toolErr != nil {
				errMsg = toolErr.Error()
			}
			if marshalErr != nil {
				if errMsg == "" {
					errMsg = marshalErr.Error()
				} else {
					errMsg = errMsg + "; " + marshalErr.Error()
				}
			}

			results = append(results, ToolResult{
				ToolName:   toolName,
				ToolCallID: toolCallID,
				OK:         toolErr == nil && marshalErr == nil,
				OutputJSON: string(response),
				Error:      strings.TrimSpace(errMsg),
			})
			last := results[len(results)-1]
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, toolStep, agentsdk.ToolResultEvent{
				ToolName:   last.ToolName,
				ToolCallID: last.ToolCallID,
				OK:         last.OK,
				OutputJSON: last.OutputJSON,
				Error:      last.Error,
			})

			if toolErr != nil && in.Callbacks.RecordFailure != nil {
				in.Callbacks.RecordFailure(toolName, toolCallID, call.Raw, toolErr)
			}
		}

		if len(parseRes.Repairs) > 0 {
			warnPayload := map[string]any{
				"warning":  "tool_data repaired",
				"repaired": true,
				"repairs":  parseRes.Repairs,
				"advice":   "Use well-formed <tool_data><call>... XML. Close tags correctly and prefer canonical field names.",
			}
			warnBytes, _ := json.Marshal(warnPayload)
			warnJSON := string(warnBytes)
			protoToolName := "tool_protocol"
			protoToolCallID := fmt.Sprintf("xml_%d_protocol", step)
			protoResult := ToolResult{
				ToolName:   protoToolName,
				ToolCallID: protoToolCallID,
				OK:         true,
				OutputJSON: warnJSON,
			}
			recordedCalls = append(recordedCalls, agentsdk.ToolCall{ID: protoToolCallID, Name: protoToolName})
			results = append(results, protoResult)
			emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, toolStep, agentsdk.ToolResultEvent{
				ToolName:   protoResult.ToolName,
				ToolCallID: protoResult.ToolCallID,
				OK:         protoResult.OK,
				OutputJSON: protoResult.OutputJSON,
				Error:      protoResult.Error,
			})
			trace(ctx, in.Callbacks, toolStep, "Tool protocol: repaired tool_data (see tool_protocol result)")
		}

		toolResultMsg := buildToolResultMessage(results)
		if in.Callbacks.ObserveStep != nil {
			in.Callbacks.ObserveStep(StepRecord{
				VisibleContent:    stepVisible,
				AssistantContent:  assistantForHistory,
				ToolCalls:         recordedCalls,
				ToolResults:       results,
				ToolResultMessage: toolResultMsg,
			})
		}
		msgs = append(msgs, agentsdk.Message{Role: "user", Content: toolResultMsg})
	}

	err := fmt.Errorf("xml tool call limit reached (max_steps=%d)", maxSteps)
	if in.Callbacks.RecordFailure != nil {
		in.Callbacks.RecordFailure("", "", "", err)
	}
	if in.Callbacks.OnError != nil {
		in.Callbacks.OnError(err.Error())
	}
	emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindError, max(0, maxSteps-1), agentsdk.ErrorEvent{Error: err.Error()})
	return combined.String(), err
}

func trace(ctx context.Context, callbacks Callbacks, step int, message string) {
	if callbacks.OnTrace != nil {
		callbacks.OnTrace(message)
	}
	emitEvent(ctx, callbacks.EventSink, agentsdk.EventKindTrace, step, agentsdk.TraceEvent{Message: message})
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func normalizeMaxSteps(maxSteps int) int {
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	if maxSteps < 1 {
		return 1
	}
	if maxSteps > maxMaxStepsCap {
		return maxMaxStepsCap
	}
	return maxSteps
}

func replaceLast(s, old, new string) string {
	if old == "" {
		return s
	}
	idx := strings.LastIndex(s, old)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func (in RunLoopInput) recoverTruncatedWriteFileAppend(
	ctx context.Context,
	step int,
	recovered recoveredWriteFileAppend,
	stepVisible string,
	msgs *[]agentsdk.Message,
) (combinedStep string, continued bool, err error) {
	toolName := "write_file"
	toolCallID := fmt.Sprintf("xml_%d_%d", step, 0)
	emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolCall, step, agentsdk.ToolCallEvent{
		ID:   toolCallID,
		Name: toolName,
		Fields: map[string]string{
			"filePath": recovered.FilePath,
			"content":  recovered.Content,
			"append":   "true",
		},
		Raw: "",
	})

	payload, toolErr := in.Executor.Execute(ctx, agentsdk.ToolCall{
		ID:     toolCallID,
		Name:   toolName,
		Fields: map[string]string{"filePath": recovered.FilePath, "content": recovered.Content, "append": "true"},
	})
	if toolErr != nil {
		payload = map[string]string{"error": fmt.Sprintf("Tool execution failed: %v", toolErr)}
	}

	response, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		if in.Callbacks.RecordFailure != nil {
			in.Callbacks.RecordFailure(toolName, toolCallID, recovered.RepairedAssistantContent, marshalErr)
		}
		response = []byte(fmt.Sprintf(`{"error":"failed to marshal tool response: %v"}`, marshalErr))
	}

	writeResult := ToolResult{
		ToolName:   toolName,
		ToolCallID: toolCallID,
		OK:         toolErr == nil && marshalErr == nil,
		OutputJSON: string(response),
	}
	if toolErr != nil {
		writeResult.Error = toolErr.Error()
	}
	if marshalErr != nil {
		if writeResult.Error == "" {
			writeResult.Error = marshalErr.Error()
		} else {
			writeResult.Error = writeResult.Error + "; " + marshalErr.Error()
		}
	}

	protoToolName := "tool_protocol"
	protoToolCallID := fmt.Sprintf("xml_%d_protocol", step)

	warnMsg := "truncated <tool_data> block repaired; write_file executed in append mode"
	warnPayload := map[string]any{
		"warning":     warnMsg,
		"recovered":   true,
		"tool":        toolName,
		"filePath":    recovered.FilePath,
		"append_only": true,
		"write_ok":    writeResult.OK,
		"advice":      "Continue with write_file append=true to finish remaining content; ensure </tool_data> is present and keep each chunk reasonably small.",
	}
	if !writeResult.OK && strings.TrimSpace(writeResult.Error) != "" {
		warnPayload["write_error"] = writeResult.Error
		warnPayload["warning"] = "truncated <tool_data> block repaired; attempted write_file append but tool returned error"
	}
	warnBytes, _ := json.Marshal(warnPayload)
	warnJSON := string(warnBytes)

	results := []ToolResult{
		writeResult,
		{
			ToolName:   protoToolName,
			ToolCallID: protoToolCallID,
			OK:         true,
			OutputJSON: warnJSON,
		},
	}
	for _, r := range results {
		emitEvent(ctx, in.Callbacks.EventSink, agentsdk.EventKindToolResult, step, agentsdk.ToolResultEvent{
			ToolName:   r.ToolName,
			ToolCallID: r.ToolCallID,
			OK:         r.OK,
			OutputJSON: r.OutputJSON,
			Error:      r.Error,
		})
	}

	toolResultMsg := buildToolResultMessage(results)

	trace(ctx, in.Callbacks, step, "Tool protocol: recovered truncated <tool_data> for write_file append")

	if in.Callbacks.ObserveStep != nil {
		in.Callbacks.ObserveStep(StepRecord{
			VisibleContent:    stepVisible,
			AssistantContent:  recovered.RepairedAssistantContent,
			ToolCalls:         []agentsdk.ToolCall{{ID: toolCallID, Name: toolName}, {ID: protoToolCallID, Name: protoToolName}},
			ToolResults:       results,
			ToolResultMessage: toolResultMsg,
		})
	}

	*msgs = append(*msgs,
		agentsdk.Message{Role: "assistant", Content: recovered.RepairedAssistantContent},
		agentsdk.Message{Role: "user", Content: toolResultMsg},
	)

	return "", true, nil
}

func buildToolResultMessage(results []ToolResult) string {
	var b strings.Builder
	b.WriteString("<tool_result>\n")
	for _, r := range results {
		b.WriteString("  <call>\n")
		b.WriteString("    <tool_name>")
		b.WriteString(escapeXMLText(r.ToolName))
		b.WriteString("</tool_name>\n")
		b.WriteString("    <tool_call_id>")
		b.WriteString(escapeXMLText(r.ToolCallID))
		b.WriteString("</tool_call_id>\n")
		b.WriteString("    <ok>")
		if r.OK {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteString("</ok>\n")
		if r.Error != "" {
			b.WriteString("    <error>")
			b.WriteString(escapeXMLText(r.Error))
			b.WriteString("</error>\n")
		}
		if r.OutputJSON != "" {
			b.WriteString("    <output>")
			b.WriteString(escapeXMLText(r.OutputJSON))
			b.WriteString("</output>\n")
		}
		b.WriteString("  </call>\n")
	}
	b.WriteString("</tool_result>\n")
	return b.String()
}

func escapeXMLText(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
	)
	return replacer.Replace(value)
}

func emitEvent(ctx context.Context, sink agentsdk.EventSink, kind agentsdk.EventKind, step int, payload any) {
	if sink == nil {
		return
	}
	sink.OnEvent(ctx, agentsdk.Event{
		Kind:     kind,
		Protocol: "xml",
		Step:     step,
		Time:     time.Now(),
		Payload:  payload,
	})
}

func cloneOptions(opts *agentsdk.ChatCompletionOptions) *agentsdk.ChatCompletionOptions {
	if opts == nil {
		return nil
	}
	copied := *opts
	if opts.Stop != nil {
		copied.Stop = append([]string(nil), opts.Stop...)
	}
	return &copied
}

func cloneFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		out[k] = v
	}
	return out
}
