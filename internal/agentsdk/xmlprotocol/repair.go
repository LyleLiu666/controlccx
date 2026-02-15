package xmlprotocol

// Repair describes a best-effort tool-protocol fix applied by the runtime.
// It is intended to be surfaced back to the model (e.g. via a tool_protocol result)
// to avoid the model learning malformed protocol formats from context.
type Repair struct {
	Kind     string            `json:"kind"`
	Message  string            `json:"message"`
	Severity string            `json:"severity,omitempty"` // info|warn
	Detail   map[string]string `json:"detail,omitempty"`
}
