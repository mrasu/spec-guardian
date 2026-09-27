package artifact

import (
	"fmt"
	"maps"
	"slices"

	"github.com/mrasu/spec-guardian/internal/fizzbee/pb"
)

// Link is one transition from adjacency_lists_*.pb.
//
// It corresponds to a graph.proto Link message such as:
//
//	{
//	  "src":12, "dest":13, "name":"Request#2.Process",
//	  "reqId":0, "newToOldThreads":{"1":0}, "type":"action"
//	}
type Link struct {
	// Src is the zero-based source Node index from protobuf field src.
	Src int64
	// Dst is the zero-based destination Node index from protobuf field dest.
	Dst int64
	// Name identifies the transition, such as "Request#2.Process" or "thread-0".
	Name string
	// Labels contains FizzBee graph labels from protobuf field labels.
	Labels []string
	// ReqID is the source thread index that starts or executes this transition.
	ReqID int64
	// NewToOldThreads maps each destination thread index to its source thread index.
	// For example, {1: 0} means source thread 0 became destination thread 1.
	NewToOldThreads map[int64]int64
	// Type classifies the transition; an Action start has the value "action".
	Type string
}

// NewLink validates and converts one protobuf Link.
func NewLink(link *pb.Link) (*Link, error) {
	if link == nil {
		return nil, fmt.Errorf("link is nil")
	}
	if link.ReqId < 0 {
		return nil, fmt.Errorf("link request thread identity %d is invalid", link.ReqId)
	}
	mappedThreadIDs := make(map[int64]bool, len(link.NewToOldThreads))
	for newThreadID, oldThreadID := range link.NewToOldThreads {
		if newThreadID < 0 || oldThreadID < 0 {
			return nil, fmt.Errorf("link thread mapping %d to %d is invalid", newThreadID, oldThreadID)
		}
		if mappedThreadIDs[oldThreadID] {
			return nil, fmt.Errorf("link thread mapping has multiple successors for %d", oldThreadID)
		}
		mappedThreadIDs[oldThreadID] = true
	}
	return &Link{
		Src:             link.Src,
		Dst:             link.Dest,
		Name:            link.Name,
		Labels:          slices.Clone(link.Labels),
		ReqID:           link.ReqId,
		NewToOldThreads: maps.Clone(link.NewToOldThreads),
		Type:            link.Type,
	}, nil
}
