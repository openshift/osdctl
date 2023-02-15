package command

type Command interface {
	Init() error
	Validate() error
	Run() (Output, error)
}

type Output interface {
	Print()
}
