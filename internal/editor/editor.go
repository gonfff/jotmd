package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

type Command struct {
	Executable string
	Args       []string
}

func Resolve(flagValue string, configured []string) (Command, error) {
	var parts []string
	var err error
	switch {
	case flagValue != "":
		parts, err = split(flagValue)
	case len(configured) != 0:
		parts = append([]string(nil), configured...)
	case os.Getenv("EDITOR") != "":
		parts, err = split(os.Getenv("EDITOR"))
	default:
		parts = []string{"vi"}
	}
	if err != nil {
		return Command{}, fmt.Errorf("parse editor command: %w", err)
	}
	if len(parts) == 0 || parts[0] == "" {
		return Command{}, errors.New("editor executable must not be empty")
	}
	executable, err := exec.LookPath(parts[0])
	if err != nil {
		return Command{}, fmt.Errorf("editor executable %q not found: %w", parts[0], err)
	}
	return Command{Executable: executable, Args: append([]string(nil), parts[1:]...)}, nil
}

func Exec(command Command, absolutePath string, done func(error) tea.Msg) tea.Cmd {
	args := append(append([]string(nil), command.Args...), absolutePath)
	return tea.ExecProcess(exec.Command(command.Executable, args...), done)
}

func split(value string) ([]string, error) {
	const (
		unquoted = iota
		singleQuoted
		doubleQuoted
	)
	var words []string
	var word strings.Builder
	state, escaped, started := unquoted, false, false
	for _, char := range value {
		if escaped {
			if state == doubleQuoted && char != '"' && char != '\\' && char != '$' && char != '`' && char != '\n' {
				word.WriteRune('\\')
			}
			if char != '\n' {
				word.WriteRune(char)
			}
			escaped, started = false, true
			continue
		}
		switch state {
		case singleQuoted:
			if char == '\'' {
				state = unquoted
			} else {
				word.WriteRune(char)
			}
			started = true
		case doubleQuoted:
			switch char {
			case '"':
				state = unquoted
			case '\\':
				escaped = true
			default:
				word.WriteRune(char)
			}
			started = true
		default:
			switch {
			case unicode.IsSpace(char):
				if started {
					words = append(words, word.String())
					word.Reset()
					started = false
				}
			case char == '\'':
				state, started = singleQuoted, true
			case char == '"':
				state, started = doubleQuoted, true
			case char == '\\':
				escaped, started = true, true
			default:
				word.WriteRune(char)
				started = true
			}
		}
	}
	if escaped {
		return nil, errors.New("trailing escape")
	}
	if state != unquoted {
		return nil, errors.New("unterminated quote")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}
