package gol

import (
	"fmt"
	"net/rpc"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	KeyPressed <-chan rune
}

func StoreWorld(p Params, c distributorChannels, filename string) [][]byte {
	//get input and filename
	c.ioCommand <- ioInput
	c.ioFilename <- filename
	//Create a 2D slice to store the world.
	twoD := make([][]byte, p.ImageHeight)
	for i := 0; i < p.ImageHeight; i++ {
		twoD[i] = make([]byte, p.ImageWidth)
		for j := 0; j < p.ImageWidth; j++ {
			twoD[i][j] = <-c.ioInput
		}
	}
	return twoD
}

func CreatEmptyWorld(imageHeight, imageWidth int) [][]byte {
	world := make([][]byte, imageHeight)
	for i := range world {
		world[i] = make([]byte, imageWidth)
	}
	return world
}

func CalculateAliveCells(p Params, world [][]byte) []util.Cell {
	var aliveCells []util.Cell
	for h := 0; h < p.ImageHeight; h++ {
		for w := 0; w < p.ImageWidth; w++ {
			if world[h][w] == 255 {
				aliveCells = append(aliveCells, util.Cell{X: w, Y: h})
			}
		}
	}
	return aliveCells
}

func outputImage(p Params, c distributorChannels, turn int, world [][]byte, filename string) {
	c.ioCommand <- ioOutput
	c.ioFilename <- filename
	for h := 0; h < p.ImageHeight; h++ {
		for w := 0; w < p.ImageWidth; w++ {
			output := world[h][w]
			c.ioOutput <- output
		}
	}
	c.events <- ImageOutputComplete{
		CompletedTurns: turn,
		Filename:       filename,
	}
}

func GetFlippedCell(p Params, world, newWorld [][]byte) []util.Cell {
	var FlippedCell []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] != newWorld[y][x] {
				FlippedCell = append(FlippedCell, util.Cell{X: x, Y: y})
			}
		}
	}
	return FlippedCell
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	//RPC CAll
	client, _ := rpc.Dial("tcp", "127.0.0.1:8030")
	defer client.Close()

	// Create a 2D slice to store the world.
	filename := fmt.Sprintf("%vx%v", p.ImageHeight, p.ImageWidth)
	initialWorld := StoreWorld(p, c, filename)
	newWorld := CreatEmptyWorld(p.ImageHeight, p.ImageWidth)

	turn := 0
	aliveCells := 0
	tickerMutex := &sync.Mutex{}
	tickerCompleted := make(chan bool)

	ifPause := make(chan bool)
	pause := false
	//quit := false

	//sent initial world to broker
	request := stubs.RequestToBroker{World: initialWorld, Params: stubs.Params(p), Stop: false}
	response := new(stubs.ResponseFromBroker)
	go client.Call(stubs.RunAllTurns, request, response)

	go func() {
		for {
			//Ticker : every two second out put the alive cells
			ticker := time.NewTicker(2 * time.Second)
			select {

			case <-ticker.C:
				tickerMutex.Lock()
				if !pause {
					c.events <- AliveCellsCount{CompletedTurns: turn, CellsCount: aliveCells}
				}
				tickerMutex.Unlock()

			case <-tickerCompleted:
				ticker.Stop()
				return

			case key := <-c.KeyPressed:
				switch key {
				case 's':
					outputImage(p, c, turn, initialWorld, "press-s")
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle

				case 'q':
					outputImage(p, c, turn, initialWorld, "press-q")
					//if pause {
					//	ifPause <- true
					//}
					//quit = true
					pause = true
					request := stubs.RequestQuit{Quit: true}
					response := new(stubs.ResponseFromBroker)
					client.Call(stubs.Quit, request, response)
					fmt.Println("Quitting")
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
					close(c.events)
				case 'p':
					if pause {
						ifPause <- true
						pause = false
						fmt.Println("Continuing the turn: ", turn)
						c.events <- StateChange{CompletedTurns: turn, NewState: Executing}
					} else {
						pause = true
						fmt.Println("Paused on the turn: ", turn)
						c.events <- StateChange{CompletedTurns: turn, NewState: Paused}
					}
				case 'k':
					outputImage(p, c, turn, initialWorld, "press-k")
					pause = true
					request := stubs.RequestToBroker{Stop: true}
					response := new(stubs.ResponseFromBroker)
					client.Call(stubs.RunAllTurns, request, response)
					fmt.Println("golWorker and broker Stop")

					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
					close(c.events)
				}

			}
		}
	}()

	//update data from broker
	for {
		if turn >= p.Turns {
			break
		}

		//if quit {
		//	break
		//}

		//keyboard control
		if pause {
			<-ifPause
		}

		req := stubs.RequestNewData{}
		res := new(stubs.ResponseFromBroker)
		client.Call(stubs.GetNewData, req, res)

		// get response from golWorker
		newWorld = res.NewWorld

		//SDL Live View
		var cells []util.Cell
		if turn == 0 {
			cells = CalculateAliveCells(p, newWorld)
		} else {
			cells = GetFlippedCell(p, initialWorld, newWorld)
		}
		for cell := 0; cell < len(cells); cell++ {
			c.events <- CellFlipped{CompletedTurns: turn, Cell: cells[cell]}
		}

		//upload for next turn
		tickerMutex.Lock()
		initialWorld = newWorld
		aliveCells = res.AliveNumber
		c.events <- TurnComplete{CompletedTurns: turn}
		turn = res.Turn
		tickerMutex.Unlock()
	}

	// Report the final state using FinalTurnCompleteEvent.

	tickerCompleted <- true

	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: CalculateAliveCells(p, initialWorld)}
	outImageName := fmt.Sprintf("%vx%vx%v", p.ImageHeight, p.ImageWidth, turn)
	outputImage(p, c, turn, initialWorld, outImageName)
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
