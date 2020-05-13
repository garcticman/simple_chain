package bc

import "fmt"

type ValidationError struct {
	address string
	numOfBlock uint64
	error error
}

//todo прямо отлично с ошибками! прочитал про ошибки - это значения?
func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s has sent incorrect block %d with next error:\n\t%s", ve.address, ve.numOfBlock, ve.error)
}

type BlockMessageError struct {
	error string
}

func (bme BlockMessageError) Error() string {
	return bme.error
}

type OrderError struct {
	address string
	block Block
}

func (oe OrderError) Error() string {
	return fmt.Sprintf("block from %s not applyed, not next in order", oe.address)
}

type CreationBlockError struct {
	error string
}

func (cbe CreationBlockError) Error() string {
	return cbe.error
}