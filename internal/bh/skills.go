// Package bh scripted reusable procedures. Each skill is a named markdown file
// with a front-matter block: name, description, and steps.
package bh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill is a named, documented procedure.
type Skill struct {
	Name        string
	Description string
	Steps       []string
}

// Skills loads and indexes skill files from a directory.
type Skills struct {
	dir    string
	skills map[string]Skill
}

// NewSkills loads skills from dir (empty if missing).
func NewSkills(dir string) *Skills {
	s := &Skills{dir: dir, skills: map[string]Skill{}}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return s
	}
	s.load()
	return s
}

func (s *Skills) load() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skill, err := parseSkill(filepath.Join(s.dir, e.Name()))
		if err == nil {
			s.skills[name] = skill
		}
	}
}

// parseSkill parses a skill markdown file with front-matter.
func parseSkill(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	skill, _ := parseSkillBody(string(data))
	return skill, nil
}

// parseSkillBody extracts the skill from a markdown file with front matter.
func parseSkillBody(content string) (Skill, string) {
	content = strings.TrimLeft(content, "\n")
	if len(content) >= 6 && content[0] == '-' && content[1] == '-' && content[2] == '-' {
		idx := strings.Index(content[3:], "---")
		if idx >= 0 {
			front := content[3 : 3+idx]
			body := strings.TrimSpace(content[3+idx+3:])
			return parseFront(front), body
		}
	}
	return Skill{}, ""
}

func parseFront(front string) Skill {
	s := Skill{Name: filepath.Base(strings.TrimSpace(front)), Steps: []string{}}
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "name:"):
			s.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		case strings.HasPrefix(line, "desc:"):
			s.Description = strings.TrimSpace(strings.TrimPrefix(line, "desc:"))
		case strings.HasPrefix(line, "steps:"):
			rest := strings.TrimSpace(strings.TrimPrefix(line, "steps:"))
			if rest != "" {
				s.Steps = append(s.Steps, rest)
			}
		}
	}
	return s
}

// List returns the names of all loaded skills.
func (s *Skills) List() []string {
	names := make([]string, 0, len(s.skills))
	for n := range s.skills {
		names = append(names, n)
	}
	return names
}

// Get returns a skill by name.
func (s *Skills) Get(name string) (Skill, bool) {
	skill, ok := s.skills[name]
	return skill, ok
}

// Render renders a skill's steps as a markdown string.
func (s *Skills) Render(name string) (string, bool) {
	skill, ok := s.Get(name)
	if !ok {
		return "", false
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Skill: %s\n", skill.Name))
	if skill.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", skill.Description))
	}
	b.WriteString("Steps:\n")
	for _, step := range skill.Steps {
		b.WriteString("- " + step + "\n")
	}
	return b.String(), true
}
