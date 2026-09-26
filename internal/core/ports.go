package core

// Interfaces are declared where they are consumed, not where they are
// implemented, so each collaborator asks for exactly as much as it uses.
//
// *Service satisfies all four; nothing in this package refers to the concrete
// type, so a test can supply a fake downloader and a fake clock.

// Adder is the intake the browser extension and the popup use.
type Adder interface {
	Add(Req) (Job, error)
}

// Lister is all the GUI needs to draw its table.
type Lister interface {
	List() []Job
}

// Rev is the change feed the GUI redraws from. The channel returned closes the
// moment any job changes, and each call returns a fresh one — so a redraw loop
// can never miss an update.
type Rev interface {
	Rev() <-chan struct{}
}

// Controller is the mutating surface behind the toolbar buttons and the
// HTTP API. Every method is variadic so "act on the selection" and
// "act on everything" are the same code path.
type Controller interface {
	Start(ids ...string) error
	Pause(ids ...string) error
	Stop(ids ...string) error
	Remove(ids ...string) error
}
