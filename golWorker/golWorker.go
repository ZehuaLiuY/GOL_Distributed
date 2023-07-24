package main

import (
	"flag"
	"math/rand"
	"net"
	"net/rpc"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
)

type GolWorker struct {
	listener net.Listener
}

func (w *GolWorker) mod(x, m int) int {
	return (x + m) % m
}

func (w *GolWorker) CalculateNextWorld(req stubs.RequestToWorker, res *stubs.ResponseFromWorker) (err error) {
	if req.Stop {
		w.listener.Close()
		return
	}
	alive := 0
	for y := 1; y <= req.ImageHeight; y++ {
		for x := 0; x < req.ImageWidth; x++ {
			//calculate around live cells
			lives := 0
			for i := -1; i <= 1; i++ {
				for j := -1; j <= 1; j++ {
					if i != 0 || j != 0 {
						if req.World[y+i][w.mod(x+j, req.ImageWidth)] == 255 {
							lives++
						}
					}
				}
			}

			//get the new world count alive cells
			if req.World[y][x] == 255 {
				if lives == 2 || lives == 3 {
					req.NewWorld[y-1][x] = 255
					alive++
				} else {
					req.NewWorld[y-1][x] = 0
				}
			} else {
				if lives == 3 {
					req.NewWorld[y-1][x] = 255
					alive++
				} else {
					req.NewWorld[y-1][x] = 0
				}
			}
		}
	}
	//upload data
	res.NewWorld = req.NewWorld
	res.Turns = req.Turns + 1
	res.AliveNumber = alive
	return
}

func main() {
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	golWorker := &GolWorker{}
	golWorker.listener, _ = net.Listen("tcp", ":"+*pAddr)
	rpc.Register(golWorker)
	defer golWorker.listener.Close()
	rpc.Accept(golWorker.listener)
}
