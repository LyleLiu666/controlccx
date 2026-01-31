package runsafe

type Intent string

const (
	IntentAnalyze      Intent = "analyze"
	IntentCode         Intent = "code"
	IntentSearchBrowse Intent = "search-browse"
	IntentInstall      Intent = "install"
)

type SafetyEnvelope string

const (
	EnvelopeDefault        SafetyEnvelope = "default"
	EnvelopeInstallEnabled SafetyEnvelope = "install-enabled"
)

type Decision struct {
	Intent     Intent
	Confidence float64
	Signals    []string
	Reason     string
}

