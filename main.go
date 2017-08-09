/*******************************************************************************
*
* Copyright 2017 Stefan Majewsky <majewsky@gmx.net>
*
* This program is free software: you can redistribute it and/or modify it under
* the terms of the GNU General Public License as published by the Free Software
* Foundation, either version 3 of the License, or (at your option) any later
* version.
*
* This program is distributed in the hope that it will be useful, but WITHOUT ANY
* WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
* A PARTICULAR PURPOSE. See the GNU General Public License for more details.
*
* You should have received a copy of the GNU General Public License along with
* this program. If not, see <http://www.gnu.org/licenses/>.
*
*******************************************************************************/

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Actor struct {
	Name          string
	Label         string
	DisplayOrder  uint
	Activities    []*Activity
	BlockedByCall string //during parsing, contains name of not-yet-answered synchronous message
	ActivityCount uint   //during parsing, counts number of running activities
}

type Activity struct {
	StartTime uint
	StopTime  uint
	//layout parameters
	Layer uint
}

type Message struct {
	Kind         string //command name that generated the message (one of "send", "call", "return")
	Label        string
	SenderName   string
	ReceiverName string
	SenderTime   uint
	ReceiverTime uint
	//layout parameters
	SenderLayer   uint
	ReceiverLayer uint
}

const (
	HeaderHeight          = 50
	SwimlaneStep          = 25  //per unit of time
	SwimlaneWidth         = 200 //per actor
	LabelWidth            = SwimlaneWidth / 2
	LabelHeight           = 20
	ActivityWidth         = 20
	ActivityOffset        = ActivityWidth / 2
	ArrowTipSize          = 10
	MessageFontSize       = 12
	MessageBaselineOffset = 3
)

func main() {
	actors, messages := parse()

	/* enable this for debugging * /
	for name, actor := range actors {
		fmt.Fprintf(os.Stderr, "actor %s = %#v\n", name, actor)
		for idx, activity := range actor.Activities {
			fmt.Fprintf(os.Stderr, "activity %d = %#v\n", idx, activity)
		}
	}
	for name, message := range messages {
		fmt.Fprintf(os.Stderr, "message %s = %#v\n", name, message)
	}
	/* */

	maxTime := getMaxTime(actors)
	width := len(actors) * SwimlaneWidth
	height := HeaderHeight + SwimlaneStep*(maxTime+2)
	fmt.Printf(`<svg version="1.1" baseProfile="full" xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`,
		width, height)

	fmt.Printf(`
		<defs>
			<marker id="normal" viewBox="0 0 10 10" refX="1" refY="5" markerWidth="%d" markerHeight="%d" orient="auto">
				<path d="M 0 0 L 10 5 L 0 5 L 10 5 L 0 10" fill="none" stroke="black" />
			</marker>
			<marker id="filled" viewBox="0 0 10 10" refX="1" refY="5" markerWidth="%d" markerHeight="%d" orient="auto">
				<path d="M 0 0 L 10 5 L 0 10 z" fill="black" />
			</marker>
		</defs>
	`, ArrowTipSize, ArrowTipSize, ArrowTipSize, ArrowTipSize)

	for _, actor := range actors {
		actor.drawSwimLane(maxTime)
		for _, activity := range actor.Activities {
			activity.drawBox(actor.DisplayOrder)
		}
	}
	for _, message := range messages {
		message.drawArrow(actors[message.SenderName], actors[message.ReceiverName])
	}

	fmt.Println(`</svg>`)
}

////////////////////////////////////////////////////////////////////////////////
// parsing

func parse() (actors map[string]*Actor, messages map[string]*Message) {
	actors = make(map[string]*Actor)
	messages = make(map[string]*Message)

	r := bufio.NewReader(os.Stdin)
	var time uint = 1

	loop := true
	for loop {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			loop = false //break after this iteration
		} else {
			failIfErr(err)
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			//advance time on every empty line
			time++
			continue
		}

		switch fields[0] {
		case "start":
			parseStart(fields[1:], time, actors)
		case "stop":
			parseStop(fields[1:], time, actors)
		case "label":
			parseLabel(fields[1:], actors)
		case "send", "call", "return":
			parseSend(fields[1:], fields[0], time, actors, messages)
		case "receive":
			parseReceive(fields[1:], time, actors, messages)
		default:
			fail("unknown command: %s", fields[0])
		}
	}

	for _, actor := range actors {
		if actor.ActivityCount > 0 {
			fail("actor %s has %d unfinished activities", actor.Name, actor.ActivityCount)
		}
	}
	for name, message := range messages {
		if message.ReceiverName == "" {
			fail("message %s was not received by anyone", name)
		}
	}

	return
}

func makeActor(name string, actors map[string]*Actor) *Actor {
	actor, exists := actors[name]
	if !exists {
		actor = &Actor{Name: name, Label: name, DisplayOrder: uint(len(actors))}
		actors[name] = actor
	}
	return actor
}

func parseStart(args []string, time uint, actors map[string]*Actor) {
	if len(args) != 1 {
		fail("wrong number of arguments for 'start': expected 1, got %d", len(args))
	}
	actor := makeActor(args[0], actors)
	activity := &Activity{StartTime: time, Layer: actor.ActivityCount}
	actor.Activities = append(actor.Activities, activity)
	actor.ActivityCount++
}

func parseStop(args []string, time uint, actors map[string]*Actor) {
	if len(args) != 1 {
		fail("wrong number of arguments for 'start': expected 1, got %d", len(args))
	}
	actor := makeActor(args[0], actors)

	var activityToStop *Activity
	for _, a := range actor.Activities {
		if a.StopTime == 0 {
			activityToStop = a
		}
	}
	if activityToStop == nil {
		fail("cannot stop actor %s: not active", actor.Name)
	}

	activityToStop.StopTime = time
	actor.ActivityCount--
}

func parseLabel(args []string, actors map[string]*Actor) {
	if len(args) < 2 {
		fail("wrong number of arguments for 'label': expected 2, got %d", len(args))
	}
	actor := makeActor(args[0], actors)
	actor.Label = strings.Join(args[1:], " ")
}

func parseSend(args []string, kind string, time uint, actors map[string]*Actor, messages map[string]*Message) {
	if len(args) < 3 {
		fail("wrong number of arguments for '%s': expected 3, got %d", kind, len(args))
	}
	sender := makeActor(args[0], actors)

	name := args[1]
	if _, exists := messages[name]; exists {
		fail("cannot send message %s multiple times", name)
	}
	if sender.BlockedByCall != "" {
		fail("actor %s cannot send message %s while waiting for response to %s", sender.Name, name, sender.BlockedByCall)
	}

	if sender.ActivityCount == 0 {
		fail("actor %s cannot send message %s while not active", sender.Name, name)
	}

	messages[name] = &Message{
		Kind:        kind,
		Label:       strings.Join(args[2:], " "),
		SenderName:  sender.Name,
		SenderTime:  time,
		SenderLayer: sender.ActivityCount - 1,
	}
	switch kind {
	case "call":
		sender.BlockedByCall = name
	case "return":
		parseStop([]string{sender.Name}, time, actors)
	}
}

func parseReceive(args []string, time uint, actors map[string]*Actor, messages map[string]*Message) {
	if len(args) != 2 {
		fail("wrong number of arguments for 'stop': expected 2, got %d", len(args))
	}
	receiver := makeActor(args[0], actors)
	name := args[1]
	msg, exists := messages[name]
	if !exists {
		fail("cannot receive message %s: has not been sent yet", name)
	}

	if receiver.BlockedByCall == "" {
		if msg.Kind == "return" {
			fail("actor %s cannot receive return message without having made a call", receiver.Name)
		}
	} else {
		if msg.Kind != "return" {
			fail("actor %s cannot receive message %s while waiting for response to %s",
				receiver.Name, name, receiver.BlockedByCall)
		}
		called := messages[receiver.BlockedByCall].ReceiverName
		if called != msg.SenderName {
			fail("actor %s cannot receive response to message %s from actor %s (expected actor %s)",
				receiver.Name, receiver.BlockedByCall, msg.SenderName, called,
			)
		}
		receiver.BlockedByCall = ""
	}

	if msg.Kind == "call" {
		parseStart([]string{receiver.Name}, time, actors)
	}

	if receiver.ActivityCount == 0 {
		fail("actor %s cannot receive message %s while not active", receiver.Name, name)
	}

	msg.ReceiverName = receiver.Name
	msg.ReceiverTime = time
	msg.ReceiverLayer = receiver.ActivityCount - 1
}

////////////////////////////////////////////////////////////////////////////////
// layout calculations

func getMaxTime(actors map[string]*Actor) (max uint) {
	for _, actor := range actors {
		for _, activity := range actor.Activities {
			if max < activity.StopTime {
				max = activity.StopTime
			}
		}
	}
	return
}

////////////////////////////////////////////////////////////////////////////////
// rendering

func (actor *Actor) drawSwimLane(maxTime uint) {
	x := actor.DisplayOrder*SwimlaneWidth + SwimlaneWidth/2
	fmt.Printf(`<rect x="%d" y="%d" width="%d" height="%d" stroke="black" fill="white" />`,
		x-LabelWidth/2, HeaderHeight-LabelHeight, LabelWidth, LabelHeight,
	)
	fmt.Printf(`<text x="%d" y="%g" font-size="%g" text-anchor="middle">%s</text>`,
		x, HeaderHeight-0.25*LabelHeight, 0.7*LabelHeight, actor.Label,
	)
	fmt.Printf(`<line x1="%d" x2="%d" y1="%d" y2="%d" stroke="black" stroke-dasharray="5,5" />`,
		x, x, HeaderHeight, HeaderHeight+(maxTime+1)*SwimlaneStep,
	)
}

func (activity *Activity) drawBox(actorDisplayOrder uint) {
	x := actorDisplayOrder*SwimlaneWidth + SwimlaneWidth/2 + activity.Layer*ActivityOffset
	yStart := HeaderHeight + SwimlaneStep*activity.StartTime
	yStop := HeaderHeight + SwimlaneStep*activity.StopTime
	fmt.Printf(`<rect x="%d" y="%d" width="%d" height="%d" stroke="black" fill="white" />`,
		x-ActivityWidth/2, yStart, ActivityWidth, yStop-yStart,
	)
}

func (message *Message) drawArrow(sender *Actor, receiver *Actor) {
	x1 := sender.DisplayOrder*SwimlaneWidth + SwimlaneWidth/2 + message.SenderLayer*ActivityOffset
	x2 := receiver.DisplayOrder*SwimlaneWidth + SwimlaneWidth/2 + message.ReceiverLayer*ActivityOffset
	y1 := HeaderHeight + SwimlaneStep*message.SenderTime
	y2 := HeaderHeight + SwimlaneStep*message.ReceiverTime
	var xText uint
	if sender.DisplayOrder < receiver.DisplayOrder {
		x1 += ActivityWidth / 2
		x2 -= ActivityWidth / 2
		x2 -= ArrowTipSize
		xText = sender.DisplayOrder*SwimlaneWidth + SwimlaneWidth
	} else {
		x1 -= ActivityWidth / 2
		x2 += ActivityWidth / 2
		x2 += ArrowTipSize
		xText = sender.DisplayOrder * SwimlaneWidth
	}

	opts := ""
	if message.Kind == "return" {
		opts += `stroke-dasharray="5,5"`
	}
	marker := "normal"
	if message.Kind == "call" {
		marker = "filled"
	}

	fmt.Printf(`<line x1="%d" x2="%d" y1="%d" y2="%d" stroke="black" marker-end="url(#%s)" %s/>`,
		x1, x2, y1, y2, marker, opts,
	)
	//TODO: use <textPath> for asynchronous messages
	fmt.Printf(`<text x="%d" y="%d" font-size="%d" text-anchor="middle">%s</text>`,
		xText, y1-MessageBaselineOffset, MessageFontSize, message.Label,
	)
}

////////////////////////////////////////////////////////////////////////////////
// utilities

func fail(msg string, args ...interface{}) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func failIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
