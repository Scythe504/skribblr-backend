package internal

import "math"

// NEW: Pixel art data structures
type GridPosition struct {
	GridX int `json:"gridX"`
	GridY int `json:"gridY"`
}

type PixelData struct {
	Type      string `json:"type"`
	X         string `json:"x"`
	Y         string `json:"y"`
	Color     string `json:"color"`
	Timestamp int64  `json:"timestamp"`
}

type PixelBatch struct {
	Type      string         `json:"type"` // "batch_place" | "batch_erase"
	Pixels    []GridPosition `json:"pixels"`
	Color     string         `json:"color"`
	Timestamp int64          `json:"timestamp"`
}

// Union type for pixel messages - we'll handle this in the message parsing
type PixelMessage struct {
	// Single pixel operations
	Type      PixelMessageType `json:"type"`
	X         *int             `json:"x,omitempty"`
	Y         *int             `json:"y,omitempty"`
	Color     string           `json:"color,omitempty"`
	Timestamp int64            `json:"timestamp"`
	Pixels    []GridPosition   `json:"pixels,omitempty"` // Batch operations
}

type PixelMessageType string

const (
	BatchPlace PixelMessageType = "batch_place"
	PixelPlace PixelMessageType = "pixel"
	ErasePixel PixelMessageType = "erase"
	BatchErase PixelMessageType = "batch_erase"
)

const (
	CanvasWidth  = 35
	CanvasHeight = 20
)

func NormalizeCoordinates(x int, y int, clientCanvasWidth int, clientCanvasHeight int) (gridX int, gridY int) {
	// - Assume client sends coordinates scaled to their canvas size
	// - Convert to server grid
	gridX = int(math.Floor(float64(x) * float64(CanvasWidth) / float64(clientCanvasWidth)))
	gridY = int(math.Floor(float64(y) * float64(CanvasHeight) / float64(clientCanvasHeight)))

	// - Clamp to grid bounds
	if gridX < 0 {
		gridX = 0
	} else if gridX >= CanvasWidth {
		gridX = CanvasWidth - 1
	}
	if gridY < 0 {
		gridY = 0
	} else if gridY >= CanvasHeight {
		gridY = CanvasHeight - 1
	}

	return
}
