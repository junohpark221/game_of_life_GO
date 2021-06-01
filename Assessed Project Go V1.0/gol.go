package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Functiomn to perform the standard C type modulo for negative numbers
func modulo(d, m int) int {
	var res = d % m
	if (res < 0 && m > 0) || (res > 0 && m < 0) {
		return res + m
	}
	return res
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p golParams, d distributorChans, alive chan []cell) {
	// Create the 2D slice to store the world.
	world := make([][]byte, p.imageHeight)
	for i := range world {
		world[i] = make([]byte, p.imageWidth)
	}

	// Create a second 2D slice to store changes.
	worldupdator := make([][]byte, p.imageHeight)
	for i := range worldupdator {
		worldupdator[i] = make([]byte, p.imageWidth)
	}

	// Request the io goroutine to read in the image with the given filename.
	d.io.command <- ioInput
	d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

	// The io goroutine sends the requested image byte by byte, in rows.
	for y := 0; y < p.imageHeight; y++ {
		for x := 0; x < p.imageWidth; x++ {
			val := <-d.io.inputVal
			if val != 0 {
				fmt.Println("Alive cell at", x, y)
				world[y][x] = val
			}
		}
	}

	// Calculate the new state of Game of Life after the given number of turns.
	for turns := 0; turns < p.turns; turns++ {
		for y := 0; y < p.imageHeight; y++ {
			for x := 0; x < p.imageWidth; x++ {
				// Sum the number of adjacent alive cells wrapping around at the edges.
				alive := 0
				for i := -1; i < 2; i++ {
					for j := -1; j < 2; j++ {
						if world[modulo(y+i, p.imageHeight)][modulo(x+j, p.imageWidth)] != 0 {
							alive++
						}
					}
				}
				if world[y][x] != 0 {
					alive--
				}

				// Populate the worldupdator slice to store all staged changes.
				if world[y][x] != 0 {
					if alive < 2 {
						worldupdator[y][x] = 0xFF
					} else {
						if alive > 3 {
							worldupdator[y][x] = 0xFF
						}
					}
				} else {
					if alive == 3 {
						worldupdator[y][x] = 0xFF
					}
				}
			}
		}

		// wirte pgm out
		d.io.command <- ioOutput
		d.io.filename <- strings.Join([]string{strconv.Itoa(p.imageWidth), strconv.Itoa(p.imageHeight)}, "x")

		// Exclusively or the changes onto the world from the worldupdator.
		for y := 0; y < p.imageHeight; y++ {
			for x := 0; x < p.imageWidth; x++ {
				world[y][x] = world[y][x] ^ worldupdator[y][x]
				d.io.inputVal <- world[y][x]
				worldupdator[y][x] = 0
			}
		}
	}

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
