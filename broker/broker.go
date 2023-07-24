package main

import (
	"flag"
	"math/rand"
	"net"
	"net/rpc"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
)

type Data struct {
	Height   int
	Width    int
	World    [][]byte
	NewWorld [][]byte
}

type Result struct {
	NewWorld [][]byte
	AliveNum int
	Turn     int
}

type Broker struct {
	listener net.Listener
	world    [][]byte
	aliveNum int
	turn     int
	quit     bool
}

func CreateEmptyWorld(imageHeight, imageWidth int) [][]byte {
	world := make([][]byte, imageHeight)
	for i := range world {
		world[i] = make([]byte, imageWidth)
	}
	return world
}

func AllocateToWorker(wholeData Data, thread int, data []Data) []Data {
	//fmt.Println("thread", thread)
	height := wholeData.Height / thread
	for t := 0; t < thread; t++ {
		data[t].Width = wholeData.Width
		if t != thread-1 {
			data[t].Height = height
		} else {
			data[t].Height = wholeData.Height - (height * (thread - 1))
		}
		//set suitable newWorld
		for y := 0; y < data[t].Height; y++ {
			for x := 0; x < data[t].Width; x++ {
				data[t].NewWorld[y][x] = wholeData.NewWorld[y][x] //do not matter the data inside each NewWorld
			}
		}
		//separate the world
		for y := 0; y <= data[t].Height+1; y++ {
			for x := 0; x < data[t].Width; x++ {
				if (height*t - 1 + y) > wholeData.Height-1 {
					data[t].World[y][x] = wholeData.World[0][x]
				} else if (height*t - 1 + y) < 0 {
					data[t].World[y][x] = wholeData.World[wholeData.Height-1][x]
				} else {
					data[t].World[y][x] = wholeData.World[(height*t-1)+y][x]
				}
			}
		}
	}
	return data
}

func GetResult(results []Result, r Result, p stubs.Params) Result {
	Range := p.ImageHeight / p.Threads
	alive := 0
	for t := 0; t < p.Threads; t++ {

		if t == p.Threads-1 {
			for y := 0; y < p.ImageHeight-(Range*(p.Threads-1)); y++ {
				for x := 0; x < p.ImageWidth; x++ {
					r.NewWorld[(Range*(p.Threads-1))+y][x] = results[t].NewWorld[y][x]
				}
			}
		} else {
			for y := 0; y < Range; y++ {
				for x := 0; x < p.ImageWidth; x++ {
					r.NewWorld[((Range)*t)+y][x] = results[t].NewWorld[y][x]
				}
			}
		}
		alive += results[t].AliveNum
		r.Turn = results[t].Turn
	}
	r.AliveNum = alive
	return r
}

// sync the turn
var complete = make(chan bool)

func (b *Broker) GetNewData(req stubs.RequestNewData, res *stubs.ResponseFromBroker) (err error) {
	<-complete
	res.NewWorld = b.world

	res.Turn = b.turn
	res.AliveNumber = b.aliveNum

	return
}

var quit = make(chan bool)

func (b *Broker) Quit(req stubs.RequestQuit, res *stubs.ResponseFromBroker) (err error) {
	quit <- req.Quit
	return
}

func (b *Broker) RunAllTurns(req stubs.RequestToBroker, res *stubs.ResponseFromBroker) (err error) {
	client, _ := rpc.Dial("tcp", "127.0.0.1:8040")
	defer client.Close()

	//press "K", stop listen
	if req.Stop {
		request := stubs.RequestToWorker{Stop: true}
		response := new(stubs.ResponseFromWorker)
		client.Call(stubs.CalculateNextWorld, request, response)
		b.listener.Close()
		return
	}

	//initialise

	b.turn = 0
	b.aliveNum = 0
	b.world = req.World
	b.quit = false
	newWorld := CreateEmptyWorld(req.Params.ImageHeight, req.Params.ImageWidth)
	var waitGroup2 sync.WaitGroup

	WholeData := Data{
		World:    b.world,
		NewWorld: newWorld,
		Height:   req.Params.ImageHeight,
		Width:    req.Params.ImageWidth,
	}
	data := make([]Data, req.Params.Threads)

	wholeResult := Result{
		NewWorld: newWorld,
		AliveNum: b.aliveNum,
		Turn:     b.turn,
	}
	results := make([]Result, req.Params.Threads)

	//Initial the list
	for t := 0; t < req.Params.Threads; t++ {
		h := req.Params.ImageHeight / req.Params.Threads
		if t == req.Params.Threads-1 {
			h = req.Params.ImageHeight - ((req.Params.Threads - 1) * (req.Params.ImageHeight / req.Params.Threads))
		}
		data[t].World = CreateEmptyWorld(h+2, req.Params.ImageWidth) //add halo region in the world
		data[t].NewWorld = CreateEmptyWorld(h, req.Params.ImageWidth)
		results[t].NewWorld = CreateEmptyWorld(h, req.Params.ImageWidth)
	}

	//main loop execute all turns
	for {
		if b.turn >= req.Params.Turns {
			break
		}
		WholeData.World = b.world
		//fmt.Println("Whole world: ", WholeTask.World)
		data = AllocateToWorker(WholeData, req.Params.Threads, data)
		for t := 0; t < req.Params.Threads; t++ {
			waitGroup2.Add(1)
			request := stubs.RequestToWorker{
				World:       data[t].World,
				NewWorld:    data[t].NewWorld,
				Turns:       b.turn,
				ImageHeight: data[t].Height,
				ImageWidth:  data[t].Width,
				Stop:        false,
			}
			response := new(stubs.ResponseFromWorker)
			client.Call(stubs.CalculateNextWorld, request, response)
			results[t].NewWorld = response.NewWorld
			results[t].AliveNum = response.AliveNumber
			results[t].Turn = response.Turns
			waitGroup2.Done()
		}
		waitGroup2.Wait()

		wholeResult = GetResult(results, wholeResult, req.Params)
		newWorld = wholeResult.NewWorld
		b.world = newWorld
		b.aliveNum = wholeResult.AliveNum
		b.turn = wholeResult.Turn
		//fmt.Println(b.turn)
		complete <- true //Channel Synchronization
		select {
		case <-quit:
			return
		default:
		}
	}
	return
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	broker := &Broker{}
	broker.listener, _ = net.Listen("tcp", ":"+*pAddr)
	rpc.Register(broker)
	defer broker.listener.Close()
	rpc.Accept(broker.listener)
}
