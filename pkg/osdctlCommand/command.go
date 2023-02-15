package command

type Command interface {
	Validate() error
	Run() error
}

type Output interface {
	Print()
}
