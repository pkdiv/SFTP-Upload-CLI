package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type Writer struct {
	mu  sync.Mutex
	out io.Writer
}

func New(out io.Writer) *Writer {
	return &Writer{out: out}
}

func (w *Writer) Printf(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintf(w.out, format, args...)
}

func (w *Writer) Println(args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintln(w.out, args...)
}

func (w *Writer) Header(title string) {
	w.Println(title)
	w.Println(strings.Repeat("─", 36))
}

func (w *Writer) Section(label, value string) {
	w.Printf("%s:\n  %s\n", label, value)
}

func (w *Writer) BlankLine() {
	w.Println()
}


