package client

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"testing"
)

func TestDefaultLogger(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	msg := "test message"
	lgr := NewDefaultLogger()
	lgr.Logf(msg)

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	_ = w.Close()
	os.Stdout = old
	out := <-outC

	assert.Equal(t, msg+"\n", out[len(out)-len(msg)-1:])
}

func TestLoggerFunc_Logf(t *testing.T) {
	lgr := LoggerFunc(func(format string, args ...interface{}) {
		assert.Equal(t, format, "format")
		assert.Len(t, args, 2)
		assert.Equal(t, "arg1", args[0])
		assert.Equal(t, "arg2", args[1])
	})
	lgr.Logf("format", "arg1", "arg2")
}
