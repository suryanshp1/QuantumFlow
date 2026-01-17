package inference

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// StreamDisplay handles the visual display of streaming tokens
type StreamDisplay struct {
	writer       io.Writer
	buffer       strings.Builder
	mu           sync.Mutex
	tokens       int
	startTime    time.Time
	lastUpdate   time.Time
	updateDelay  time.Duration
	enableColors bool
}

// NewStreamDisplay creates a new stream display
func NewStreamDisplay(writer io.Writer, enableColors bool) *StreamDisplay {
	return &StreamDisplay{
		writer:       writer,
		updateDelay:  10 * time.Millisecond, // Smooth typewriter effect
		enableColors: enableColors,
		startTime:    time.Now(),
		lastUpdate:   time.Now(),
	}
}

// Write writes a token to the display
func (s *StreamDisplay) Write(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.WriteString(token)
	s.tokens++

	// Rate-limit updates for smoother display
	now := time.Now()
	if now.Sub(s.lastUpdate) >= s.updateDelay {
		_, err := fmt.Fprint(s.writer, token)
		s.lastUpdate = now
		return err
	}

	return nil
}

// WriteAll writes a complete response (non-streaming)
func (s *StreamDisplay) WriteAll(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.WriteString(text)
	tokens := len(strings.Fields(text))
	s.tokens += tokens

	_, err := fmt.Fprintln(s.writer, text)
	return err
}

// Finalize flushes any remaining buffered content and displays stats
func (s *StreamDisplay) Finalize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Flush any buffered content
	if s.buffer.Len() > 0 {
		fmt.Fprintln(s.writer)
	}

	// Display statistics
	duration := time.Since(s.startTime)
	tokensPerSec := 0.0
	if duration.Seconds() > 0 {
		tokensPerSec = float64(s.tokens) / duration.Seconds()
	}

	if s.enableColors {
		fmt.Fprintf(s.writer, "\n\033[90m⏱ %.2fs | 🚀 %.1f tokens/s | 📝 %d tokens\033[0m\n",
			duration.Seconds(), tokensPerSec, s.tokens)
	} else {
		fmt.Fprintf(s.writer, "\n[%.2fs | %.1f tokens/s | %d tokens]\n",
			duration.Seconds(), tokensPerSec, s.tokens)
	}

	return nil
}

// GetContent returns the accumulated content
func (s *StreamDisplay) GetContent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

// Reset resets the display state
func (s *StreamDisplay) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.Reset()
	s.tokens = 0
	s.startTime = time.Now()
	s.lastUpdate = time.Now()
}

// ColorCode represents ANSI color codes
type ColorCode string

const (
	ColorReset   ColorCode = "\033[0m"
	ColorRed     ColorCode = "\033[31m"
	ColorGreen   ColorCode = "\033[32m"
	ColorYellow  ColorCode = "\033[33m"
	ColorBlue    ColorCode = "\033[34m"
	ColorMagenta ColorCode = "\033[35m"
	ColorCyan    ColorCode = "\033[36m"
	ColorGray    ColorCode = "\033[90m"
	ColorBold    ColorCode = "\033[1m"
)

// Colorize wraps text in color codes if colors are enabled
func Colorize(text string, color ColorCode, enabled bool) string {
	if !enabled {
		return text
	}
	return string(color) + text + string(ColorReset)
}

// ProgressIndicator shows a simple progress animation
type ProgressIndicator struct {
	writer   io.Writer
	message  string
	frames   []string
	current  int
	stopChan chan struct{}
	mu       sync.Mutex
}

// NewProgressIndicator creates a new progress indicator
func NewProgressIndicator(writer io.Writer, message string) *ProgressIndicator {
	return &ProgressIndicator{
		writer:   writer,
		message:  message,
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		stopChan: make(chan struct{}),
	}
}

// Start starts the progress animation
func (p *ProgressIndicator) Start() {
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.mu.Lock()
				frame := p.frames[p.current%len(p.frames)]
				fmt.Fprintf(p.writer, "\r%s %s", frame, p.message)
				p.current++
				p.mu.Unlock()
			case <-p.stopChan:
				fmt.Fprintf(p.writer, "\r\033[K") // Clear line
				return
			}
		}
	}()
}

// Stop stops the progress animation
func (p *ProgressIndicator) Stop() {
	close(p.stopChan)
	time.Sleep(100 * time.Millisecond) // Allow final clear to complete
}

// UpdateMessage updates the progress message
func (p *ProgressIndicator) UpdateMessage(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.message = message
}
