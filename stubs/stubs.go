package stubs

var CalculateNextWorld = "GolWorker.CalculateNextWorld"
var GetNewData = "Broker.GetNewData"
var RunAllTurns = "Broker.RunAllTurns"
var Quit = "Broker.Quit"

type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type RequestToWorker struct {
	World       [][]byte
	NewWorld    [][]byte
	Turns       int
	ImageHeight int
	ImageWidth  int
	Stop        bool
}

type ResponseFromWorker struct {
	NewWorld    [][]byte
	Turns       int
	AliveNumber int
}

type RequestToBroker struct {
	Params Params
	World  [][]byte
	Stop   bool //shutdown
}

type ResponseFromBroker struct {
	NewWorld    [][]byte
	Turn        int
	AliveNumber int
}

type RequestNewData struct {
}

type RequestQuit struct {
	Quit bool // quit
}
