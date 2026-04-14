package collector

type Snapshot struct {
	Online        bool    `json:"online"`
	SSHOK         bool    `json:"sshOk"`
	CPUUsage      float64 `json:"cpuUsage"`
	MemoryUsage   float64 `json:"memoryUsage"`
	DiskUsage     float64 `json:"diskUsage"`
	OSVersion     string  `json:"osVersion"`
	KernelVersion string  `json:"kernelVersion"`
	UptimeSeconds int64   `json:"uptimeSeconds"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	Source        string  `json:"source"`
}
