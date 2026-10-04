package streamclient

import (
	"fmt"
	"time"

	"pebblepost/internal/types"
)

// LogCallback is called whenever a new log entry is produced.
type LogCallback func(entry types.StreamLogEntry)

// StatusCallback is called whenever session state changes.
type StatusCallback func(status types.StreamSessionStatus)

// Standard WebSocket close codes (RFC 6455).
const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseProtocolError           = 1002
	CloseUnsupportedData         = 1003
	CloseNoStatusReceived        = 1005
	CloseAbnormalClosure         = 1006
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseMessageTooBig           = 1009
	CloseMandatoryExtension      = 1010
	CloseInternalServerError     = 1011
	CloseTLSHandshake            = 1015
)

// CloseCodeDescription returns human-readable text for a WebSocket close code.
func CloseCodeDescription(code int) string {
	switch code {
	case CloseNormalClosure:
		return "Normal Closure (1000)"
	case CloseGoingAway:
		return "Going Away (1001)"
	case CloseProtocolError:
		return "Protocol Error (1002)"
	case CloseUnsupportedData:
		return "Unsupported Data (1003)"
	case CloseNoStatusReceived:
		return "No Status Received (1005)"
	case CloseAbnormalClosure:
		return "Abnormal Closure (1006)"
	case CloseInvalidFramePayloadData:
		return "Invalid Frame Payload Data (1007)"
	case ClosePolicyViolation:
		return "Policy Violation (1008)"
	case CloseMessageTooBig:
		return "Message Too Big (1009)"
	case CloseMandatoryExtension:
		return "Mandatory Extension (1010)"
	case CloseInternalServerError:
		return "Internal Server Error (1011)"
	case CloseTLSHandshake:
		return "TLS Handshake Failed (1015)"
	default:
		if code > 0 {
			return fmt.Sprintf("Close Code %d", code)
		}
		return ""
	}
}

// MakeSystemLog creates a system-level StreamLogEntry.
func MakeSystemLog(index int, logType string, payload string, isError bool) types.StreamLogEntry {
	return types.StreamLogEntry{
		ID:        fmt.Sprintf("log_%d_%d", time.Now().UnixNano(), index),
		Index:     index,
		Direction: "system",
		Type:      logType,
		Timestamp: time.Now(),
		Payload:   payload,
		Size:      len(payload),
		IsError:   isError,
	}
}
