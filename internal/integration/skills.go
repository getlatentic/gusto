package integration

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

var skillName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// readSkills finds each skill: a folder directly under skills/ that holds a SKILL.md. Folders deeper down are not searched.
func (p *Plugin) readSkills(root string) {
	top := filepath.Join(root, "skills")
	if _, err := os.Lstat(top); err != nil {
		return
	}
	if !directory(root, top) {
		p.leave("skills", "it is not a folder, so it holds no skills")
		return
	}
	entries, err := os.ReadDir(top)
	if err != nil {
		p.leave("skills", "%v", err)
		return
	}
	for _, entry := range entries {
		folder := filepath.Join(top, entry.Name())
		where := "skills/" + entry.Name()
		file := filepath.Join(folder, "SKILL.md")
		if _, err := os.Lstat(file); err != nil {
			continue
		}
		if !directory(root, folder) || !regular(root, file) {
			p.leave(where, "its SKILL.md is not a file inside the plugin")
			continue
		}
		if _, _, escape, err := measure(root, folder); err != nil || escape != "" {
			p.leave(where, "it holds a link that leaves the plugin's folder")
			continue
		}
		skill, problem := readSkill(file, entry.Name())
		if problem != "" {
			p.leave(where, "%s", problem)
			continue
		}
		skill.Dir = where
		p.Skills = append(p.Skills, skill)
	}
	sort.Slice(p.Skills, func(a, b int) bool { return p.Skills[a].Name < p.Skills[b].Name })
}

type frontMatter struct {
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	License       string         `yaml:"license"`
	Compatibility string         `yaml:"compatibility"`
	Metadata      map[string]any `yaml:"metadata"`
	AllowedTools  string         `yaml:"allowed-tools"`
}

// readSkill reads a SKILL.md as the Agent Skills specification asks, and says what is wrong with it in one sentence, or nothing.
func readSkill(path, folder string) (Skill, string) {
	text, err := os.ReadFile(path)
	switch {
	case err != nil:
		return Skill{}, err.Error()
	case len(text) > maxSkillFile:
		return Skill{}, fmt.Sprintf("SKILL.md is larger than %d KiB", maxSkillFile>>10)
	case !utf8.Valid(text):
		return Skill{}, "SKILL.md is not UTF-8 text"
	}
	front, found := frontOf(text)
	if !found {
		return Skill{}, "SKILL.md has no front matter between --- lines"
	}
	var meta frontMatter
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return Skill{}, "its front matter is not YAML: " + firstLine(err.Error())
	}
	if problem := checkSkill(meta, folder); problem != "" {
		return Skill{}, problem
	}
	skill := Skill{Name: meta.Name, Description: meta.Description, License: meta.License, Compatibility: meta.Compatibility, AllowedTools: meta.AllowedTools}
	if len(meta.Metadata) > 0 {
		skill.Metadata = map[string]string{}
		for key, value := range meta.Metadata {
			skill.Metadata[key] = fmt.Sprint(value)
		}
	}
	return skill, ""
}

func checkSkill(meta frontMatter, folder string) string {
	switch {
	case len(meta.Name) < 1 || len(meta.Name) > 64 || !skillName.MatchString(meta.Name):
		return "its name must be 1 to 64 lower case letters, digits and single hyphens, not starting or ending with a hyphen"
	case meta.Name != folder:
		return fmt.Sprintf("its name %q is not the name of its folder %q", meta.Name, folder)
	case strings.TrimSpace(meta.Description) == "":
		return "it has no description, which is how an agent decides to use it"
	case utf8.RuneCountInString(meta.Description) > 1024:
		return "its description is longer than 1024 characters"
	case utf8.RuneCountInString(meta.Compatibility) > 500:
		return "its compatibility is longer than 500 characters"
	}
	return ""
}

// frontOf is the text between the first two --- lines, which must begin the file.
func frontOf(text []byte) ([]byte, bool) {
	text = bytes.TrimPrefix(text, []byte("\xef\xbb\xbf"))
	text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
	rest, found := bytes.CutPrefix(text, []byte("---\n"))
	if !found {
		return nil, false
	}
	front, _, found := bytes.Cut(rest, []byte("\n---"))
	return front, found
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
