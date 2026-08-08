package sandbox

// CreateScanTaskRequest is sent to POST /analysis/createScanTask after upload.
type CreateScanTaskRequest struct {
	FileURI     string       `json:"file_uri"`
	FileName    string       `json:"file_name,omitempty"`
	AsyncResult bool         `json:"async_result,omitempty"`
	ShortResult *bool        `json:"short_result,omitempty"`
	Options     *ScanOptions `json:"options,omitempty"`
}

type ScanOptions struct {
	AnalysisDepth      int             `json:"analysis_depth,omitempty"`
	PasswordsForUnpack []string        `json:"passwords_for_unpack,omitempty"`
	Sandbox            *SandboxOptions `json:"sandbox,omitempty"`
}

type SandboxOptions struct {
	Enabled                 bool     `json:"enabled,omitempty"`
	ImageID                 string   `json:"image_id,omitempty"`
	FileTypes               []string `json:"file_types,omitempty"`
	AnalysisDuration        int      `json:"analysis_duration,omitempty"`
	BootkitMonitor          bool     `json:"bootkitmon,omitempty"`
	BootkitAnalysisDuration int      `json:"analysis_duration_bootkitmon,omitempty"`
	SaveVideo               *bool    `json:"save_video,omitempty"`
	ManInTheMiddleEnabled   bool     `json:"mitm_enabled,omitempty"`
}

type Response[T any] struct {
	Data   T          `json:"data"`
	Errors []APIError `json:"errors"`
}

type APIError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

type ScanData struct {
	ScanID    string     `json:"scan_id"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Result    ScanResult `json:"result,omitempty"`
}

type Artifact struct {
	Artifacts      []Artifact      `json:"artifacts,omitempty"`
	EngineResults  []EngineResult  `json:"engine_results,omitempty"`
	FileInfo       FileInfo        `json:"file_info,omitempty"`
	NetworkObjects []NetworkObject `json:"network_objects,omitempty"`
	Result         ScanResult      `json:"result,omitempty"`
	Type           string          `json:"type,omitempty"`
}

type EngineResult struct {
	DatabaseTime    float64     `json:"database_time,omitempty"`
	DatabaseVersion string      `json:"database_version,omitempty"`
	Detections      []Detection `json:"detections,omitempty"`
	EngineCodeName  string      `json:"engine_code_name,omitempty"`
	EngineSubsystem string      `json:"engine_subsystem,omitempty"`
	EngineVersion   string      `json:"engine_version,omitempty"`
	Result          ScanResult  `json:"result,omitempty"`
}

type Detection struct {
	Detect string `json:"detect,omitempty"`
	Threat string `json:"threat,omitempty"`
}

type FileInfo struct {
	FilePath string `json:"file_path,omitempty"`
	FileURI  string `json:"file_uri,omitempty"`
	MD5      string `json:"md5,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	SHA1     string `json:"sha1,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Size     uint64 `json:"size,omitempty"`
}

type NetworkObject struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

type ScanResult struct {
	Duration     float64    `json:"duration,omitempty"`
	DurationFull float64    `json:"duration_full,omitempty"`
	Errors       []APIError `json:"errors,omitempty"`
	ScanState    string     `json:"scan_state,omitempty"`
	Threat       string     `json:"threat,omitempty"`
	Verdict      string     `json:"verdict,omitempty"`
}
