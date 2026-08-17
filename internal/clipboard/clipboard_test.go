package clipboard

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type fakeCommand struct {
	pipe                       io.WriteCloser
	pipeErr, startErr, waitErr error
}

func (f *fakeCommand) StdinPipe() (io.WriteCloser, error) { return f.pipe, f.pipeErr }
func (f *fakeCommand) Start() error                       { return f.startErr }
func (f *fakeCommand) Wait() error                        { return f.waitErr }

type fakeWriteCloser struct {
	writeErr, closeErr error
	text               strings.Builder
}

func (f *fakeWriteCloser) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.text.Write(p)
}
func (f *fakeWriteCloser) Close() error { return f.closeErr }

func TestCopyPlatformsAndErrors(t *testing.T) {
	originalOS, originalLook, originalCommand := clipboardGOOS, clipboardLookPath, newCommand
	t.Cleanup(func() { clipboardGOOS, clipboardLookPath, newCommand = originalOS, originalLook, originalCommand })
	if originalCommand("unused") == nil {
		t.Fatal("default command factory returned nil")
	}

	writer := &fakeWriteCloser{}
	created := ""
	newCommand = func(name string, args ...string) clipboardCommand {
		created = name + " " + strings.Join(args, " ")
		return &fakeCommand{pipe: writer}
	}
	for _, platform := range []string{"darwin", "windows"} {
		clipboardGOOS = platform
		if err := Copy("hello"); err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
	}
	if writer.text.String() != "hellohello" || created == "" {
		t.Fatalf("writer=%q command=%q", writer.text.String(), created)
	}

	clipboardGOOS = "plan9"
	if err := Copy("x"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatal(err)
	}

	clipboardGOOS = "linux"
	for _, available := range []string{"xclip", "xsel", "wl-copy"} {
		clipboardLookPath = func(name string) (string, error) {
			if name == available {
				return "/bin/" + name, nil
			}
			return "", errors.New("missing")
		}
		if err := Copy("x"); err != nil {
			t.Fatalf("%s: %v", available, err)
		}
		if !strings.HasPrefix(created, available+" ") {
			t.Fatalf("created %q", created)
		}
	}
	clipboardLookPath = func(string) (string, error) { return "", errors.New("missing") }
	if err := Copy("x"); err == nil || !strings.Contains(err.Error(), "no clipboard utility") {
		t.Fatal(err)
	}

	cases := []struct {
		cmd  *fakeCommand
		want string
	}{
		{&fakeCommand{pipeErr: errors.New("pipe")}, "stdin pipe"},
		{&fakeCommand{pipe: &fakeWriteCloser{}, startErr: errors.New("start")}, "start clipboard"},
		{&fakeCommand{pipe: &fakeWriteCloser{writeErr: errors.New("write")}}, "write to clipboard"},
		{&fakeCommand{pipe: &fakeWriteCloser{closeErr: errors.New("close")}}, "close stdin"},
		{&fakeCommand{pipe: &fakeWriteCloser{}, waitErr: errors.New("wait")}, "command failed"},
	}
	clipboardGOOS = "darwin"
	for _, tc := range cases {
		newCommand = func(string, ...string) clipboardCommand { return tc.cmd }
		if err := Copy("x"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
}
