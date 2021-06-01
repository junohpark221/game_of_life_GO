package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxthreads = 8

var ticking = true
var totalalive [maxthreads + 1]int

// Function to perform the standard C type modulo for negative numbers
func modulo(d, m int) int {
	var res = d % m
	if (res < 0 && m > 0) || (res > 0 && m < 0) {
		return res + m
	}
	return res
}

//Function to send the world to the PGM generator through the io channel.
func printPGM(p golParams, d distributorChans, world [][]byte) {
	d.io.command <- ioOutput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			d.io.inputVal <- world[y][x]
		}
	}
}

func ticker() {
	count := 0
	for ticking {
		for i := 0; i < maxthreads; i++ {
			count += totalalive[i]
		}
		fmt.Print("Total alive cells is: ")
		fmt.Println(count)
		count = 0
		time.Sleep(2 * time.Second)
	}
}

// Worker function to perform GOL logic on a slice of the input, returns result through chan.
func worker(p golParams, io chan [16]byte, z int) {
	var localtotalalive int
	worldslice := make([][]byte, ((p.imageHeight / p.threads) + 2))
	for i := range worldslice {
		worldslice[i] = make([]byte, p.imageWidth)
	}
	worldupdator := make([][]byte, ((p.imageHeight / p.threads) + 2))
	for i := range worldupdator {
		worldupdator[i] = make([]byte, p.imageWidth)
	}
	for {
		localtotalalive = 0
		for y := 0; y < ((p.imageHeight / p.threads) + 2); y++ {
			for x := 0; x < p.imageWidth/16; x++ {
				section := <-io
				for i := 0; i < 16; i++ {
					worldslice[y][16*x+i] = section[i]
					if section[i] != 0 {
						localtotalalive++
					}
				}
			}
		}
		for y := 1; y < ((p.imageHeight / p.threads) + 1); y++ {
			for x := 0; x < p.imageWidth; x++ {
				// Sum the number of adjacent alive cells wrapping around at the edges.
				alive := 0
				for i := -1; i < 2; i++ {
					for j := -1; j < 2; j++ {
						if worldslice[y+i][modulo(x+j, p.imageWidth)] != 0 {
							alive++
						}
					}
				}
				if worldslice[y][x] != 0 {
					alive--
				}
				// Populate the worldupdator slice to store all staged changes.
				if worldslice[y][x] != 0 {
					if alive < 2 {
						worldupdator[y][x] = 0xFF
						localtotalalive--
					} else {
						if alive > 3 {
							worldupdator[y][x] = 0xFF
							localtotalalive--
						}
					}
				} else {
					if alive == 3 {
						worldupdator[y][x] = 0xFF
						localtotalalive++
					}
				}
			}
		}
		for y := 1; y < ((p.imageHeight / p.threads) + 1); y++ {
			for x := 0; x < p.imageWidth; x++ {
				worldslice[y][x] = worldslice[y][x] ^ worldupdator[y][x]
				worldupdator[y][x] = 0
			}
		}
		for y := 1; y < ((p.imageHeight / p.threads) + 1); y++ {
			for x := 0; x < p.imageWidth/16; x++ {
				var section [16]byte
				for k := 0; k < 16; k++ {
					section[k] = worldslice[y][x*16+k]
				}
				io <- section
			}
		}
		totalalive[z] = localtotalalive
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, chans [maxthreads]chan [16]byte) {
	keypress := make(chan rune, 5)
	var paused bool
	turn := 0
	go getKeyboardCommand(keypress)
	// Create the 2D slice to store the world.
	world := make([][]byte, p.imageHeight)
	for i := range world {
		world[i] = make([]byte, p.imageWidth)
	}
	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")
	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				totalalive[maxthreads]++
				fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}
		}
	}
	fmt.Print("Total alive input cells: ")
	fmt.Println(totalalive[maxthreads])
	for turn < p.turns {
		select {
		case key := <-keypress:
			if key == 0x70 {
				paused = true
				ticking = false
				fmt.Print("Paused on turn ")
				fmt.Println(turn)
				for paused {
					select {
					case key2 := <-keypress:
						if key2 == 0x70 {
							paused = false
							ticking = true
							go ticker()
							fmt.Println("Continuing")
						}
					}
				}
			} else if key == 0x73 {
				fmt.Println("Outputting PGM file")
				printPGM(p, d, world)
			} else if key == 0x71 {
				fmt.Println("Aborting")
				turn = p.turns + 1
			}
		default:
			// Send world to workers
			for n := 0; n < p.threads; n++ {
				for y := -1; y <= p.imageHeight/p.threads; y++ {
					var section [16]byte
					for i := 0; i < p.imageWidth/16; i++ {
						for k := 0; k < 16; k++ {
							section[k] = world[modulo(n*(p.imageHeight/p.threads)+y, p.imageHeight)][i*16+k]
						}
						chans[n] <- section
					}
				}
			}
			// merge the results of the workers into a new world version.
			for n := 0; n < p.threads; n++ {
				for y := 0; y < (p.imageHeight / p.threads); y++ {
					for x := 0; x < p.imageWidth/16; x++ {
						section := <-chans[n]
						for i := 0; i < 16; i++ {
							world[(n*(p.imageHeight/p.threads))+y][16*x+i] = section[i]
						}
					}
				}
			}
		}
		turn++
	}
	// stop ticker thread
	ticking = false
	// write pgm out
	printPGM(p, d, world)
	// Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y][x] != 0 {
				finalAlive = append(finalAlive, cell{x: x, y: y})
			}
		}
	}
	// Make sure that the Io has finished any output before exiting.
	d.io.command <- ioCheckIdle
	<-d.io.idle
	// Return the coordinates of cells that are still alive.
	alive <- finalAlive
}
