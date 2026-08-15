package confirmation

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out}
}

func (p *Prompter) Ask(prompt string) (bool, error) {
	for {
		fmt.Fprintf(p.out, "%s [y/N]: ", prompt)
		line, err := p.in.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("read input: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.out, "Please answer y or n.")
		}
	}
}

func (p *Prompter) AskString(prompt string, current string) (string, error) {
	display := prompt
	if current != "" {
		display = fmt.Sprintf("%s [%s]: ", prompt, current)
	} else {
		display = prompt + ": "
	}
	fmt.Fprint(p.out, display)
	line, err := p.in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read input: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" && current != "" {
		return current, nil
	}
	return line, nil
}

func (p *Prompter) AskStringDefault(prompt string, def string) (string, error) {
	line, err := p.AskString(prompt, def)
	if err != nil {
		return "", err
	}
	if line == "" {
		return def, nil
	}
	return line, nil
}

func (p *Prompter) AskIntDefault(prompt string, def int) (int, error) {
	line, err := p.AskString(prompt, fmt.Sprintf("%d", def))
	if err != nil {
		return 0, err
	}
	if line == "" {
		return def, nil
	}
	var n int
	if _, err := fmt.Sscanf(line, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid integer %q", line)
	}
	return n, nil
}
