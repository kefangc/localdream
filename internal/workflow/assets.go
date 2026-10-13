package workflow

// CountRange is the inclusive asset range advertised by a workflow node.
type CountRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// AssetCapability describes reference limits returned by MCP capabilities.
type AssetCapability struct {
	Images   CountRange `json:"images"`
	Videos   CountRange `json:"videos"`
	Audios   CountRange `json:"audios"`
	MinTotal int        `json:"min_total"`
}
