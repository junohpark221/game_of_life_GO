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

// Function to send the world to the PGM generator through the io channel.
func printPGM(p golParams, d distributorChans, world [][]byte, padding int) {
	d.io.command <- ioOutput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			d.io.inputVal <- world[y+padding][x]
		}
	}
}

// Function to print the number of alive cells every 2 seconds.
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
func merger(p golParams, world [][]byte, chans [maxthreads]chan []byte, newimageheight int, notifiers [maxthreads]chan bool) {
	for i := 0; i < p.threads; i++ {
		notifiers[i] <- true
	}
	for n := 0; n < p.threads; n++ {
		for y := 0; y < (newimageheight / p.threads); y++ {
			for x := 0; x < p.imageWidth/16; x++ {
				section := <-chans[n]
				for i := 0; i < 16; i++ {
					world[(n*(newimageheight/p.threads))+y][16*x+i] = section[i]
				}
			}
		}
	}
}

// Function to import worldslice from the io channel.
func importer(worldslice [][]byte, p golParams, newimageheight int, localtotalalive int, io chan []byte) int {
	localtotalalive = 0
	for y := 0; y < ((newimageheight / p.threads) + 2); y++ {
		for x := 0; x < p.imageWidth/16; x++ {
			section := <-io
			for i := 0; i < 16; i++ {
				worldslice[y][16*x+i] = section[i]
				if section[i] == 0xFF {
					localtotalalive++
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
func gollogic(worldslice [][]byte, worldupdator [][]byte, p golParams, newimageheight int, padding int, localtotalalive int) int {
	for y := 1; y < ((newimageheight / p.threads) + 1); y++ {
		for x := 0; x < p.imageWidth; x++ {
			if worldslice[y][x] != 0x80 {
				alive := 0
				for i := -1; i < 2; i++ {
					for j := -1; j < 2; j++ {
						if worldslice[y+i][modulo(x+j, p.imageWidth)] == 0x80 {
							if worldslice[modulo(y+i-padding, newimageheight/p.threads)][modulo(x+j, p.imageWidth)] != 0 {
								alive++
							}
						} else if worldslice[y+i][modulo(x+j, p.imageWidth)] != 0 {
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

// Function to apply updates to the worldslice.
func updator(worldslice [][]byte, worldupdator [][]byte, p golParams, newimageheight int) {
	for y := 1; y < ((newimageheight / p.threads) + 1); y++ {
		for x := 0; x < p.imageWidth; x++ {
			worldslice[y][x] = worldslice[y][x] ^ worldupdator[y][x]
			worldupdator[y][x] = 0
		}
	}
}

// Function to return the worldslice to the distributor.
func sender(worldslice [][]byte, p golParams, newimageheight int, io chan []byte) {
	for y := 1; y < ((newimageheight / p.threads) + 1); y++ {
		for x := 0; x < p.imageWidth/16; x++ {
			var section []byte
			for k := 0; k < 16; k++ {
				section = append(section, worldslice[y][x*16+k])
			}
			io <- section
		}
	}
}

// Function to split and send the world to workers.
func exporter(world [][]byte, p golParams, newimageheight int, padding int, chans [maxthreads]chan []byte) {
	for n := 0; n < p.threads; n++ {
		for y := -1; y <= newimageheight/p.threads; y++ {
			var section []byte
			for i := 0; i < p.imageWidth/16; i++ {
				for k := 0; k < 16; k++ {
					if (n+1)*(newimageheight/p.threads) <= padding {
						if n == 0 && y == -1 {
							section = append(section, world[0][i*16+k])
						} else {
							section = append(section, world[modulo(n*(newimageheight/p.threads)+y, newimageheight)][i*16+k])
						}
					} else if padding < (n+1)*(newimageheight/p.threads) && (n+1)*(newimageheight/p.threads) <= (padding+newimageheight/p.threads) && y == -1 {
						section = append(section, world[newimageheight-1][i*16+k])
					} else if n == p.threads-1 && y == newimageheight/p.threads {
						section = append(section, world[padding][i*16+k])
					} else {
						section = append(section, world[modulo(n*(newimageheight/p.threads)+y, newimageheight)][i*16+k])
					}
				}
				chans[n] <- section
			}
		}
	}
}

// Function to perform inter-worker halo exchange.
func haloex(worldslice [][]byte, p golParams, commsup [maxthreads](chan []byte), commsdown [maxthreads](chan []byte), padding int, z int, newimageheight int) {
	firstrealworker := padding / (newimageheight / p.threads)
	firstindex := modulo(padding, newimageheight/p.threads)
	if z == firstrealworker {
		for x := 0; x < p.imageWidth/16; x++ {
			var section []byte
			for k := 0; k < 16; k++ {
				section = append(section, worldslice[firstindex+1][x*16+k])
			}
			commsup[p.threads-1] <- section
		}
	} else {
		for x := 0; x < p.imageWidth/16; x++ {
			var section []byte
			for k := 0; k < 16; k++ {
				section = append(section, worldslice[1][x*16+k])
			}
			if z == 0 {
				commsup[firstrealworker-1] <- section
			} else {
				commsup[z-1] <- section
			}
		}
	}
	for x := 0; x < p.imageWidth/16; x++ {
		section := <-commsup[z]
		for i := 0; i < 16; i++ {
			worldslice[newimageheight/p.threads+1][16*x+i] = section[i]
		}
	}
	for x := 0; x < p.imageWidth/16; x++ {
		var section []byte
		for k := 0; k < 16; k++ {
			section = append(section, worldslice[newimageheight/p.threads][x*16+k])
		}
		if z == p.threads-1 {
			commsdown[firstrealworker] <- section
		} else if z == firstrealworker-1 {
			commsdown[0] <- section
		} else {
			commsdown[z+1] <- section
		}
	}
	for x := 0; x < p.imageWidth/16; x++ {
		section := <-commsdown[z]
		for i := 0; i < 16; i++ {
			worldslice[0][16*x+i] = section[i]
		}
	}
}

// Worker function to perform GOL logic on a slice of the input, returns result through chan.
func worker(p golParams, chans [maxthreads](chan []byte), commsup [maxthreads](chan []byte), commsdown [maxthreads](chan []byte), notifier chan bool, z int) {
	var localtotalalive int
	newimageheight := p.imageHeight
	if (newimageheight % p.threads) != 0 {
		newimageheight += (p.threads - newimageheight%p.threads)
	}
	padding := newimageheight - p.imageHeight
	worldslice := make([][]byte, ((newimageheight / p.threads) + 2))
	for i := range worldslice {
		worldslice[i] = make([]byte, p.imageWidth)
	}
	worldupdator := make([][]byte, ((newimageheight / p.threads) + 2))
	for i := range worldupdator {
		worldupdator[i] = make([]byte, p.imageWidth)
	}
	localtotalalive = importer(worldslice, p, newimageheight, localtotalalive, chans[z])
	for {
		notification := <-notifier
		if notification {
			sender(worldslice, p, newimageheight, chans[z])
		}
		localtotalalive = gollogic(worldslice, worldupdator, p, newimageheight, padding, localtotalalive)
		updator(worldslice, worldupdator, p, newimageheight)
		haloex(worldslice, p, commsup, commsdown, padding, z, newimageheight)
		totalalive[z] = localtotalalive
	}
}

// Distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell, chans [maxthreads]chan []byte, notifiers [maxthreads]chan bool) {
	keypress := make(chan rune, 5)
	var paused bool
	turn := 0
	newimageheight := p.imageHeight
	if (newimageheight % p.threads) != 0 {
		newimageheight += (p.threads - newimageheight%p.threads)
	}
	padding := newimageheight - p.imageHeight
	// Start the keypress thread using the communication channel.
	go getKeyboardCommand(keypress)
	// Create the 2D slice to store the world.
	world := make([][]byte, newimageheight)
	for i := range world {
		world[i] = make([]byte, newimageheight)
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
				world[y+padding][x] = val
			}
		}
	}
	for y := 0; y < padding; y++ {
		for x := 0; x < p.imageWidth; x++ {
			world[y][x] = 0x80
		}
	}
	fmt.Printf("Total alive input cells: %d\n", totalalive[maxthreads])
	exporter(world, p, newimageheight, padding, chans)
	for turn < p.turns {
		select {
		case key := <-keypress:
			if key == 0x70 {
				paused = true
				ticking = false
				fmt.Printf("Paused on turn %d\n", turn)
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
				merger(p, world, chans, newimageheight, notifiers)
				printPGM(p, d, world, padding)
			} else if key == 0x71 {
				fmt.Println("Aborting")
				turn = p.turns + 1
			}
		default:
			for i := 0; i < p.threads; i++ {
				notifiers[i] <- false
			}
			turn++
		}
	}
	// Stop ticker thread.
	ticking = false
	// write pgm out
	merger(p, world, chans, newimageheight, notifiers)
	printPGM(p, d, world, padding)
	// Create an empty slice to store coordinates of cells that are still alive after p.turns are done.
	var finalAlive []cell
	// Go through the world and append the cells that are still alive.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			if world[y+padding][x] != 0 {
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
