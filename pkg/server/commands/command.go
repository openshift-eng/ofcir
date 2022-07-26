package commands

type command interface {
	Run() error
}
