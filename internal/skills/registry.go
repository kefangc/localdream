package skills

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"strings"
)

// Builtin skill files are shipped with Local Dream, independent of a Codex install.
//
//go:embed builtin/h3-prompt-writing
var builtin embed.FS

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

const h3Description = "Write MiniMax H3 video prompts for text, first-frame, first-and-last-frame, and full-reference creation. Use when composing H3 shots, sound, timing, or reference labels."

func List() []Skill {
	return []Skill{{Name: "h3-prompt-writing", Description: h3Description, Source: "builtin"}}
}

func Read(name, resource string) (string, error) {
	if name != "h3-prompt-writing" {
		return "", errors.New("skill not found")
	}
	if resource == "" {
		resource = "SKILL.md"
	}
	if strings.Contains(resource, "\\") || strings.HasPrefix(resource, "/") || path.Clean(resource) != resource || strings.HasPrefix(resource, "../") || resource == ".." {
		return "", errors.New("invalid skill resource path")
	}
	if resource != "SKILL.md" && resource != "references/base-en.txt" && resource != "references/ref-en.txt" {
		return "", errors.New("skill resource not found")
	}
	data, err := fs.ReadFile(builtin, "builtin/"+name+"/"+resource)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
