// Prints tz file as specified in https://tools.ietf.org/html/rfc8536
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	err := mainErr()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mainErr() error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	data, h, err := parseHeader(data)
	if err != nil {
		return err
	}
	printHeader(h)
	data, err = printDataBlock(data, h, time32)
	if err != nil {
		return err
	}
	if h.version > 1 {
		var h2 header
		data, h2, err = parseHeader(data)
		if err != nil {
			return err
		}
		printHeader(h2)
		data, err = printDataBlock(data, h2, time64)
		if err != nil {
			return err
		}
		fmt.Printf("Footer:\n%q\n", data)
	}
	return nil
}

func time32(data []byte) ([]byte, int64, error) {
	if len(data) < 4 {
		return data, 0, fmt.Errorf("missing time32 data")
	}
	value := int32(binary.BigEndian.Uint32(data[0:4]))
	return data[4:], int64(value), nil
}

func time64(data []byte) ([]byte, int64, error) {
	if len(data) < 8 {
		return data, 0, fmt.Errorf("missing time64 data")
	}
	value := int64(binary.BigEndian.Uint64(data[0:8]))
	return data[8:], value, nil
}

type header struct {
	version byte
	isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt uint32
}

func printHeader(h header) {
	fmt.Println("Header:")
	fmt.Println(" version:", h.version)
	fmt.Printf(" isutcnt: %d\n", h.isutcnt)
	fmt.Printf(" isstdcnt: %d\n", h.isstdcnt)
	fmt.Printf(" leapcnt: %d\n", h.leapcnt)
	fmt.Printf(" timecnt: %d\n", h.timecnt)
	fmt.Printf(" typecnt: %d\n", h.typecnt)
	fmt.Printf(" charcnt: %d\n", h.charcnt)
}

func parseHeader(data []byte) ([]byte, header, error) {
	var h header
	// magic
	if len(data) < 4 || !bytes.Equal(data[0:4], []byte{0x54, 0x5A, 0x69, 0x66}) {
		return data, h, fmt.Errorf("invalid header")
	}
	data = data[4:]
	// version
	if len(data) < 1 {
		return data, h, fmt.Errorf("missing version")
	}
	switch data[0] {
	case 0:
		h.version = 1
	case 0x32:
		h.version = 2
	case 0x33:
		h.version = 3
	default:
		return data, h, fmt.Errorf("unsupported version: %d", data[0])
	}
	data = data[1:]
	// unused
	if len(data) < 15 {
		return data, h, fmt.Errorf("missing unused")
	}
	data = data[15:]
	// isutcnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing isutcnt")
	}
	h.isutcnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	// isstdcnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing isstdcnt")
	}
	h.isstdcnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	// leapcnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing leapcnt")
	}
	h.leapcnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	// timecnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing timecnt")
	}
	h.timecnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	// typecnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing typecnt")
	}
	h.typecnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	// charcnt
	if len(data) < 4 {
		return data, h, fmt.Errorf("missing charcnt")
	}
	h.charcnt = binary.BigEndian.Uint32(data[:4])
	data = data[4:]
	return data, h, nil
}

func printDataBlock(data []byte, h header, timeFn func([]byte) ([]byte, int64, error)) ([]byte, error) {
	fmt.Println("Transition times:")
	for i := uint32(0); i <  h.timecnt; i++ {
		var ts int64
		var err error
		data, ts, err = timeFn(data)
		if err != nil {
			return data, err
		}
		fmt.Printf(" %d (%s UTC)\n", ts, time.Unix(ts, 0).UTC().Format("2006-01-02T15:04:05"))
	}
	fmt.Println("Transition types:")
	for i := uint32(0); i <  h.timecnt; i++ {
		if len(data) < 1 {
			return data, fmt.Errorf("missing transition type")
		}
		tt := data[0]
		if uint32(tt) > h.typecnt {
			return data, fmt.Errorf("transition type out of range")
		}
		data = data[1:]
		fmt.Printf(" %d\n", tt)
	}
	fmt.Println("Local time type records:")
	for i := uint32(0); i < h.typecnt; i++ {
		if len(data) < 6 {
			return data, fmt.Errorf("missing type record")
		}
		utoff := int32(binary.BigEndian.Uint32(data[0:4]))
		dst := data[4]
		idx := data[5]
		if uint32(idx) > h.charcnt-1 {
			return data, fmt.Errorf("idx %d out of range (0..%d)", idx, h.charcnt-1)
		}
		data = data[6:]
		fmt.Printf(" (%d) utoff=%d dst=%d idx=%d\n", i, utoff, dst, idx)
	}
	fmt.Println("Time zone designations:")
	if uint32(len(data)) < h.charcnt {
		return data, fmt.Errorf("missing time zone designations")
	}
	tzDesig := data[:h.charcnt]
	data = data[h.charcnt:]
	err := printTzDesig(tzDesig)
	if err != nil {
		return data, err
	}
	fmt.Println("Leap second records:")
	for i := uint32(0); i < h.leapcnt; i++ {
		var occur int64
		data, occur, err = timeFn(data)
		if err != nil {
			return data, err
		}
		if len(data) < 4 {
			return data, fmt.Errorf("missing corr")
		}
		corr := int32(binary.BigEndian.Uint32(data[0:4]))
		data = data[4:]
		fmt.Printf(" occur=%d corr=%d\n", occur, corr)
	}
	fmt.Println("Standard/wall indicators:")
	for i := uint32(0); i < h.isstdcnt; i++ {
		if len(data) < 1 {
			return data, fmt.Errorf("missing std/wall indicator")
		}
		switch data[0] {
		case 0:
			fmt.Printf(" (%d) wall\n", i)
		case 1:
			fmt.Printf(" (%d) standard\n", i)
		default:
			return data, fmt.Errorf("unsupported std/wall indicator: %d", data[0])
		}
		data = data[1:]
	}
	fmt.Println("UT/local indicators:")
	for i := uint32(0); i < h.isutcnt; i++ {
		if len(data) < 1 {
			return data, fmt.Errorf("missing ut/local indicator")
		}
		switch data[0] {
		case 0:
			fmt.Printf(" (%d) local\n", i)
		case 1:
			fmt.Printf(" (%d) UT\n", i)
		default:
			return data, fmt.Errorf("unsupported UT/local indicator: %d", data[0])
		}
		data = data[1:]
	}
	return data, nil
}

func printTzDesig(data []byte) error {
	start := 0
	for end := 0; end < len(data); end++ {
		if data[end] == 0 {
			fmt.Printf(" %q\n", data[start:end])
			start = end+1
		}
	}
	if start != len(data) {
		return fmt.Errorf("extra data at end of tz desig")
	}
	return nil
}
