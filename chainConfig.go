package bc

type ChainConfig struct {
	TwoSigns uint64
}

func IsForked(blockNum, head uint64) bool {
	return blockNum <= head
}
