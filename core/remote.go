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
	Fetch(remote *Remote, refsToFetch []RefToFetch) error
	Push(remote *Remote, refsToPush []RefToPush) (*[]string, error)

	Initialize(remote *Remote) error
}

type Remote struct {
	reader   io.Reader
	writer   io.Writer
	Logger   *log.Logger
	localDir string

	Repo *git.Repository

	Handler RemoteHandler

	todo      []func() (string, error)
	pushList  []func() (*[]string, error)
	fetchList []func() error
	Force     bool
}

type RefToPush struct {
	Local  string
	Remote string
}

type RefToFetch struct {
	Sha string
	Ref string
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

func (r *Remote) push(refsToPush []RefToPush) {
	r.pushList = append(r.pushList, func() (*[]string, error) {
		locals, err := r.Handler.Push(r, refsToPush)
		if err != nil {
			return nil, err
		}

		return locals, nil

		//return fmt.Sprintf("ok %s\n", done), nil, nil
	})
}

func (r *Remote) fetch(refsToFetch []RefToFetch) {
	r.fetchList = append(r.fetchList, func() error {
		return r.Handler.Fetch(r, refsToFetch)
	})
}

func (r *Remote) ProcessCommands() error {
	reader := bufio.NewReader(r.reader)
	var refsToPush []RefToPush
	var refsToFetch []RefToFetch

	prevCommand := ""
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
			refsToPush = append(refsToPush, RefToPush{
				Local:  refs[0],
				Remote: refs[1],
			})
			//r.push(refs[0], refs[1])
		case strings.HasPrefix(command, "fetch "):
			parts := strings.Split(command, " ")

			if strings.HasPrefix(parts[2], "+") {
				r.Force = true
			}

			refsToFetch = append(refsToFetch, RefToFetch{
				Sha: parts[1],
				Ref: parts[2],
			})
			// r.fetch(parts[1], parts[2])
		case command == "":
			fallthrough
		case command == "\n":
			if strings.HasPrefix(prevCommand, "push ") {
				r.push(refsToPush)
				var locals *[]string
				for _, task := range r.pushList {
					locals, err = task()
					if err != nil {
						return err
					}
				}
				if locals != nil {
					for _, local := range *locals {
						r.Printf("ok %s\n", local)
					}
					r.Printf("\n")
				}

				r.todo = nil
				break loop
			}

			if strings.HasPrefix(prevCommand, "fetch ") {
				r.fetch(refsToFetch)
				for _, task := range r.fetchList {
					err = task()
					if err != nil {
						return err
					}
				}

				r.Printf("\n")

				r.todo = nil
				break loop
			}

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

		prevCommand = command
	}

	return nil
}
