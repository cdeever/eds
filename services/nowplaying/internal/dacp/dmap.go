// Package dacp speaks the remote-control protocol iTunes and the Music app
// offer to phone remotes: DACP, which is HTTP carrying DMAP, a tagged binary
// format (ADR-0011).
//
// It pairs the way a phone remote does - a four-digit code typed into the
// player once - and never with an Apple ID or Home Sharing.
package dacp

import (
	"encoding/binary"
	"fmt"
)

// DMAP is a sequence of items: a four-character tag, a four-byte big-endian
// length, then that many bytes. Some tags are containers, whose bytes are
// more items. Nothing in the data says which, so the containers this client
// meets are listed here.
var containers = map[string]bool{
	"cmst": true, // play status
	"mlog": true, // login response
	"cmpa": true, // pairing response
	"msrv": true, // server info
	"cmgt": true, // get-property response
	"casp": true, // speakers
	"mdcl": true, // dictionary
}

// Item is one decoded DMAP item.
type Item struct {
	Tag      string
	Data     []byte
	Children []Item // set for containers
}

// maxDepth and maxItems bound what a hostile or broken peer can make this
// allocate. A real play status is a dozen items, one level deep.
const (
	maxDepth = 8
	maxItems = 4096
)

// Decode parses DMAP bytes into items.
func Decode(data []byte) ([]Item, error) {
	count := 0
	return decode(data, 0, &count)
}

func decode(data []byte, depth int, count *int) ([]Item, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("dmap: nested deeper than %d", maxDepth)
	}

	var items []Item
	for len(data) > 0 {
		if len(data) < 8 {
			return nil, fmt.Errorf("dmap: %d trailing bytes are not an item", len(data))
		}
		tag := string(data[:4])
		size := binary.BigEndian.Uint32(data[4:8])
		if uint64(size) > uint64(len(data)-8) {
			return nil, fmt.Errorf("dmap: %s claims %d bytes, %d remain", tag, size, len(data)-8)
		}
		if *count++; *count > maxItems {
			return nil, fmt.Errorf("dmap: more than %d items", maxItems)
		}

		item := Item{Tag: tag, Data: data[8 : 8+size]}
		if containers[tag] {
			children, err := decode(item.Data, depth+1, count)
			if err != nil {
				return nil, err
			}
			item.Children = children
		}
		items = append(items, item)
		data = data[8+size:]
	}
	return items, nil
}

// Find returns the first item with the given tag, searching containers too.
func Find(items []Item, tag string) (Item, bool) {
	for _, item := range items {
		if item.Tag == tag {
			return item, true
		}
		if found, ok := Find(item.Children, tag); ok {
			return found, true
		}
	}
	return Item{}, false
}

// Uint reads an item as a big-endian unsigned integer of whatever width it
// was sent in: DMAP uses one, two, four and eight bytes.
func (i Item) Uint() (uint64, bool) {
	switch len(i.Data) {
	case 1:
		return uint64(i.Data[0]), true
	case 2:
		return uint64(binary.BigEndian.Uint16(i.Data)), true
	case 4:
		return uint64(binary.BigEndian.Uint32(i.Data)), true
	case 8:
		return binary.BigEndian.Uint64(i.Data), true
	}
	return 0, false
}

// String reads an item as UTF-8 text.
func (i Item) String() string { return string(i.Data) }

// Encode builds one DMAP item.
func Encode(tag string, data []byte) []byte {
	out := make([]byte, 8+len(data))
	copy(out, tag)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(data)))
	copy(out[8:], data)
	return out
}

// EncodeUint64 builds an eight-byte integer item.
func EncodeUint64(tag string, v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return Encode(tag, buf[:])
}
