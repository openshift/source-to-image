package sti

type UsageHandler interface {
	cleanup()
	setup(required []string, optional []string) error
	execute(command string) error
}

type Usage struct {
	handler UsageHandler
}

func NewUsage(req *STIRequest) (*Usage, error) {
	h, err := newRequestHandler(req)
	if err != nil {
		return nil, err
	}
	return &Usage{handler: h}, nil
}

// Usage processes a build request by starting the container and executing
// the assemble script with a "-h" argument to print usage information
// for the script.
func (u *Usage) Usage() error {
	h := u.handler
	defer h.cleanup()

	err := h.setup([]string{"usage"}, []string{})
	if err != nil {
		return err
	}

	return h.execute("usage")
}
