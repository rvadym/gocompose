package output

import "fmt"

type Printer interface {
	Printf(format string, args ...any)
	Print(a ...any) (n int, err error)
}

var defaultPrinter = OSPrinter{}

func DefaultPrinter() Printer {
	return defaultPrinter
}

type OSPrinter struct{}

func (p OSPrinter) Printf(format string, args ...any) {
	fmt.Printf(format, args...)
}
func (p OSPrinter) Print(a ...any) (n int, err error) {
	return fmt.Print(a...)
}
