package core

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	git "github.com/go-git/go-git/v5"
)

type RemoteHandler interface {
	List(remote *Remote, forPush bool) ([]string, error)
	Fetch(remote *Remote, sha, ref string) error
	Push(remote *Remote, localRef, remoteRef string) (string, error)

	Initialize(remote *Remote) error
}

type Remote struct {
	reader   io.Reader
	writer   io.Writer
	Logger   *log.Logger
	localDir string

	Repo *git.Repository

	Handler RemoteHandler

	todo []func() (string, error)
}

func NewRemote(handler RemoteHandler, reader io.Reader, writer io.Writer, logger *log.Logger) (*Remote, error) {
	localDir, err := GetLocalDir()
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(localDir)
	if err == git.ErrWorktreeNotProvided {
		repoRoot, _ := path.Split(localDir)

		repo, err = git.PlainOpen(repoRoot)
		if err != nil {
			return nil, err
		}
	}

	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	remote := &Remote{
		reader:   reader,
		writer:   writer,
		Logger:   logger,
		localDir: localDir,

		Repo: repo,

		Handler: handler,
	}

	if err := handler.Initialize(remote); err != nil {
		return nil, err
	}

	return remote, nil
}

func (r *Remote) Printf(format string, a ...interface{}) (n int, err error) {
	// r.Logger.Printf("> "+format, a...)
	return fmt.Fprintf(r.writer, format, a...)
}

func (r *Remote) push(src, dst string) {
	r.todo = append(r.todo, func() (string, error) {
		done, err := r.Handler.Push(r, src, dst)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("ok %s\n", done), nil
	})
}

func (r *Remote) fetch(sha, ref string) {
	r.todo = append(r.todo, func() (string, error) {
		err := r.Handler.Fetch(r, sha, ref)
		if err != nil {
			return "", err
		}

		return "", nil
	})
}

func (r *Remote) ProcessCommands() error {
	reader := bufio.NewReader(r.reader)
loop:
	for {
		command, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		command = strings.Trim(command, "\n")

		// r.Logger.Printf("< %s", command)
		switch {
		case command == "capabilities":
			r.Printf("push\n")
			r.Printf("fetch\n")
			r.Printf("\n")
		case strings.HasPrefix(command, "list"):
			list, err := r.Handler.List(r, strings.HasPrefix(command, "list for-push"))
			if err != nil {
				return err
			}
			for _, e := range list {
				r.Printf("%s\n", e)
			}
			r.Printf("\n")
		case strings.HasPrefix(command, "push "):
			refs := strings.Split(command[5:], ":")
			r.push(refs[0], refs[1])
		case strings.HasPrefix(command, "fetch "):
			parts := strings.Split(command, " ")
			r.fetch(parts[1], parts[2])
		case command == "":
			fallthrough
		case command == "\n":
			for _, task := range r.todo {
				resp, err := task()
				if err != nil {
					return err
				}
				r.Printf("%s", resp)
			}
			r.Printf("\n")
			r.todo = nil
			break loop
		default:
			return fmt.Errorf("received unknown command %q", command)
		}
	}

	return nil
}
