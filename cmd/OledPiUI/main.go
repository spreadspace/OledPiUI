//
// Copyright (c) 2023 Christian Pointner <equinox@spreadspace.org>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//     * Redistributions of source code must retain the above copyright
//       notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above copyright
//       notice, this list of conditions and the following disclaimer in the
//       documentation and/or other materials provided with the distribution.
//     * Neither the name of telgo nor the names of its contributors may be
//       used to endorse or promote products derived from this software without
//       specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//

package main

import (
	"context"
	"fmt"
	"image"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/warthog618/gpiod"

	"github.com/sirupsen/logrus"
	"github.com/toxygene/gpiod-ky-040-rotary-encoder/device"

	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/devices/v3/ssd1306"
	"periph.io/x/devices/v3/ssd1306/image1bit"
	"periph.io/x/host/v3"

	"golang.org/x/image/font"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
)

func drawLine(dev *ssd1306.Dev, face font.Face, lineNum int, text string) error {
	img := image1bit.NewVerticalLSB(image.Rect(0, 0, 128, 16))
	m := face.Metrics()
	drawer := font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{image1bit.On},
		Face: face,
		Dot:  fixed.Point26_6{X: 0, Y: m.Ascent},
	}
	drawer.DrawString(text)
	height := m.Height.Round()
	yOffset := lineNum * height
	return dev.Draw(image.Rect(0, yOffset, 128, yOffset+height), drawer.Dst, image.Point{})
}

func btnHandler(evt gpiod.LineEvent) {
	fmt.Printf("btnHandler got: %+v\n", evt)
}

func main() {
	chip, err := gpiod.NewChip("gpiochip0")
	if err != nil {
		log.Fatal(err)
	}
	defer chip.Close()

	// button
	btn, err := chip.RequestLine(24, gpiod.WithPullUp, gpiod.WithEventHandler(btnHandler), gpiod.WithFallingEdge)
	if err != nil {
		log.Fatal(err)
	}
	defer btn.Close()

	// rotary encoder
	logger := logrus.New()
	logger.Out = ioutil.Discard
	// force internal Pull-Ups
	tmp, err := chip.RequestLines([]int{22, 23}, gpiod.AsInput, gpiod.WithPullUp)
	if err != nil {
		log.Fatal(err)
	}
	tmp.Close()
	actions := make(chan device.Action)
	go func() {
		defer close(actions)
		re := device.NewRotaryEncoder(chip, 22, 23, logrus.NewEntry(logger))
		err := re.Run(context.Background(), actions)
		fmt.Println("rotary-encoder go-routine has stopped with error: ", err)
	}()
	go func() {
		i := 0
		for action := range actions {
			switch action {
			case device.Clockwise:
				i++
			case device.CounterClockwise:
				i--
			}
			fmt.Printf("rotary-encoder value is now: %d\n", i)
		}
	}()

	// reset
	rst, err := chip.RequestLine(27, gpiod.AsOutput(0))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		rst.Reconfigure(gpiod.AsInput)
		rst.Close()
	}()
	rst.SetValue(1)
	time.Sleep(100 * time.Millisecond)
	rst.SetValue(0)
	time.Sleep(100 * time.Millisecond)
	rst.SetValue(1)

	// Make sure periph is initialized.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	// Use i2creg I²C bus registry to find the first available I²C bus.
	b, err := i2creg.Open("")
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	dev, err := ssd1306.NewI2C(b, &ssd1306.DefaultOpts)
	if err != nil {
		log.Fatalf("failed to initialize ssd1306: %v", err)
	}

	// Draw on it.
	if err := drawLine(dev, inconsolata.Bold8x16, 0, "Heading"); err != nil {
		log.Fatal(err)
	}
	if err := drawLine(dev, inconsolata.Regular8x16, 1, "* Menu Entry 1"); err != nil {
		log.Fatal(err)
	}
	if err := drawLine(dev, inconsolata.Regular8x16, 2, "* Menu Entry 2"); err != nil {
		log.Fatal(err)
	}
	if err := drawLine(dev, inconsolata.Regular8x16, 3, "* Menu Entry 3"); err != nil {
		log.Fatal(err)
	}

	// wait for CTRL-C
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	fmt.Printf("press CTRL-C to exit ... \n")
	<-c
}
