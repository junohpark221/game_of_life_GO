package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const maxthreads = 12

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

// Function to merge the results of worker threads.
func merger(p golParams, world [][]byte, chans [maxthreads]chan []byte, remains int) {
	for n := 0; n < p.threads; n++ {
		if n < remains {
			for y := 0; y <= (p.imageHeight / p.threads); y++ {
				for x := 0; x < p.imageWidth/16; x++ {
					section := <-chans[n]
					for i := 0; i < 16; i++ {
						world[(n*(p.imageHeight/p.threads+1))+y][16*x+i] = section[i]
					}
				}
			}
		} else {
			for y := 0; y < (p.imageHeight / p.threads); y++ {
				for x := 0; x < p.imageWidth/16; x++ {
					section := <-chans[n]
					for i := 0; i < 16; i++ {
						world[(n*(p.imageHeight/p.threads)+remains)+y][16*x+i] = section[i]
					}
				}
			}
		}
	}
}

// Function to import worldslice from the io channel.
func importer(worldslice [][]byte, p golParams, remains int, localtotalalive int, io chan []byte, z int) int {
	localtotalalive = 0
	if z < remains {
		for y := 0; y < ((p.imageHeight / p.threads) + 3); y++ {
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
	} else {
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
	}
	return localtotalalive
}

// Function to stage changes in the worldupdator slice.
func golupdatorlogic(worldslice [][]byte, worldupdator [][]byte, alive int, localtotalalive int, y int, x int) int {
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
	return localtotalalive
}

// Function to perform the GOL logic on a worker's slice, computes the alive var.
func gollogic(worldslice [][]byte, worldupdator [][]byte, p golParams, remains int, z int, localtotalalive int) int {
	if z < remains {
		for y := 1; y < ((p.imageHeight / p.threads) + 2); y++ {
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
				localtotalalive = golupdatorlogic(worldslice, worldupdator, alive, localtotalalive, y, x)
			}
		}
	} else {
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
				localtotalalive = golupdatorlogic(worldslice, worldupdator, alive, localtotalalive, y, x)
			}
		}
	}
	return localtotalalive
}

func updator(worldslice [][]byte, worldupdator [][]byte, p golParams, remains int, io chan []byte, z int) {
	if z < remains {
		for y := 1; y < ((p.imageHeight / p.threads) + 2); y++ {
			for x := 0; x < p.imageWidth; x++ {
				worldslice[y][x] = worldslice[y][x] ^ worldupdator[y][x]
				worldupdator[y][x] = 0
			}
		}
	} else {
		for y := 1; y < ((p.imageHeight / p.threads) + 1); y++ {
			for x := 0; x < p.imageWidth; x++ {
				worldslice[y][x] = worldslice[y][x] ^ worldupdator[y][x]
				worldupdator[y][x] = 0
			}
		}
	}
}

func sender(p golParams, worldslice [][]byte, io chan []byte, remains int, z int) {
	if z < remains {
		for y := 1; y < ((p.imageHeight / p.threads) + 2); y++ {
			for x := 0; x < p.imageWidth/16; x++ {
				var section []byte
				for k := 0; k < 16; k++ {
					section = append(section, worldslice[y][x*16+k])
				}
				io <- section
			}
		}
	} else {
		for y := 1; y < ((p.imageHeight / p.threads) + 1); y++ {
			for x := 0; x < p.imageWidth/16; x++ {
				var section []byte
				for k := 0; k < 16; k++ {
					section = append(section, worldslice[y][x*16+k])
				}
				io <- section
			}
		}
	}
}

func haloex(p golParams, worldslice [][]byte, io chan []byte, halosaversup [maxthreads]chan []byte, halosaversdown [maxthreads]chan []byte, remains int, z int) {
	for x := 0; x < p.imageWidth/16; x++ {
		var section []byte
		for k := 0; k < 16; k++ {
			section = append(section, worldslice[1][x*16+k])
		}
		halosaversdown[modulo(z-1, p.threads)] <- section
	}
	for x := 0; x < p.imageWidth/16; x++ {
		section := <-halosaversdown[z]
		for i := 0; i < 16; i++ {
			if z < remains {
				worldslice[(p.imageHeight/p.threads)+2][16*x+i] = section[i]
			} else {
				worldslice[(p.imageHeight/p.threads)+1][16*x+i] = section[i]
			}
		}
	}

	if z < remains {
		for x := 0; x < p.imageWidth/16; x++ {
			var section []byte
			for k := 0; k < 16; k++ {
				section = append(section, worldslice[(p.imageHeight/p.threads)+1][x*16+k])
			}
			halosaversup[modulo(z+1, p.threads)] <- section
		}
	} else {
		for x := 0; x < p.imageWidth/16; x++ {
			var section []byte
			for k := 0; k < 16; k++ {
				section = append(section, worldslice[(p.imageHeight / p.threads)][x*16+k])
			}
			halosaversup[modulo(z+1, p.threads)] <- section
		}
	}
	for x := 0; x < p.imageWidth/16; x++ {
		section := <-halosaversup[z]
		for i := 0; i < 16; i++ {
			worldslice[0][16*x+i] = section[i]
		}
	}
}

func exporter(world [][]byte, p golParams, rows int, remains int, chans [maxthreads]chan []byte) {
	for n := 0; n < p.threads; n++ {
		if n < remains {
			for y := -1; y <= rows+1; y++ {
				var section []byte
				for i := 0; i < p.imageWidth/16; i++ {
					for k := 0; k < 16; k++ {
						section = append(section, world[modulo((n*(p.imageHeight/p.threads+1))+y, p.imageHeight)][i*16+k])
					}
					chans[n] <- section
				}
			}
		} else {
			for y := -1; y <= rows; y++ {
				var section []byte
				for i := 0; i < p.imageWidth/16; i++ {
					for k := 0; k < 16; k++ {
						section = append(section, world[modulo((n*(p.imageHeight/p.threads)+remains)+y, p.imageHeight)][i*16+k])
					}
					chans[n] <- section
				}
			}
		}
	}
}

func turnreceive(p golParams, turntodis [maxthreads]chan int) {
	for n := 0; n < p.threads; n++ {
		<-turntodis[n]
	}
}

func turnchoice(p golParams, turntoworker [maxthreads]chan int, choice int) {
	for n := 0; n < p.threads; n++ {
		turntoworker[n] <- choice
	}
}

// Worker function to perform GOL logic on a slice of the input, returns result through chan.
func worker(p golParams, io chan []byte, halosaversup [maxthreads]chan []byte, halosaversdown [maxthreads]chan []byte, turntoworker chan int, turntodis chan int, z int) {
	var localtotalalive int
	running := true
	remains := p.imageHeight % p.threads
	worldslice := make([][]byte, ((p.imageHeight / p.threads) + 3))
	for i := range worldslice {
		worldslice[i] = make([]byte, p.imageWidth)
	}
	worldupdator := make([][]byte, ((p.imageHeight / p.threads) + 3))
	for i := range worldupdator {
		worldupdator[i] = make([]byte, p.imageWidth)
	}
	localtotalalive = importer(worldslice, p, remains, localtotalalive, io, z)
	for running && (p.turns > 0) {
		localtotalalive = gollogic(worldslice, worldupdator, p, remains, z, localtotalalive)
		updator(worldslice, worldupdator, p, remains, io, z)
		haloex(p, worldslice, io, halosaversup, halosaversdown, remains, z)
		totalalive[z] = localtotalalive
		turntodis <- 0
		turn := <-turntoworker
		if turn == 255 {
			running = false
		} else if turn == 73 {
			sender(p, worldslice, io, remains, z)
		} else if turn == 71 {
			sender(p, worldslice, io, remains, z)
			running = false
		} else if turn == 70 {
			paused := true
			for paused {
				select {
				case turn := <-turntoworker:
					if turn == -70 {
						paused = false
					}
				}
			}
		}
	}
	sender(p, worldslice, io, remains, z)
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, chans [maxthreads]chan []byte, turntoworker [maxthreads]chan int, turntodis [maxthreads]chan int) {
	keypress := make(chan rune, 5)
	var paused bool
	turn := 0
	rows := p.imageHeight / p.threads
	remains := p.imageHeight % p.threads
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
	exporter(world, p, rows, remains, chans)
	for turn < p.turns {
		select {
		case key := <-keypress:
			if key == 0x70 { // p
				paused = true
				ticking = false
				fmt.Print("Paused on turn ")
				fmt.Println(turn)
				turnreceive(p, turntodis)
				turnchoice(p, turntoworker, 70)
				for paused {
					select {
					case key2 := <-keypress:
						if key2 == 0x70 {
							paused = false
							ticking = true
							go ticker()
							fmt.Println("Continuing")
							turnchoice(p, turntoworker, -70)
						}
					}
				}
			} else if key == 0x73 { // s
				fmt.Println("Outputting PGM file")
				turnreceive(p, turntodis)
				turnchoice(p, turntoworker, 73)
				merger(p, world, chans, remains)
				printPGM(p, d, world)
			} else if key == 0x71 { // q
				turnreceive(p, turntodis)
				turnchoice(p, turntoworker, 71)
				merger(p, world, chans, remains)
				fmt.Println("Aborting")
				turn = p.turns + 1
			}
		default:
			turnreceive(p, turntodis)
			if turn == p.turns-1 {
				turnchoice(p, turntoworker, 255)
			} else {
				turnchoice(p, turntoworker, 0)
			}
			turn++
		}
	}
	merger(p, world, chans, remains)
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
