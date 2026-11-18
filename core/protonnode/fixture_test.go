package protonnode

import "github.com/dsh/android-proton/core/proton"

// protoServerFixture builds a minimal logical server for the selection tests.
type protoServerFixture struct {
	name string
	cc   string
}

type logicalServer = proton.LogicalServer

func (f protoServerFixture) toLogical() proton.LogicalServer {
	return proton.LogicalServer{
		Name:         f.name,
		ExitCountry:  f.cc,
		Status:       1,
		Score:        3,
		Servers: []proton.PhysicalServer{
			{ID: "p-" + f.name, EntryIP: "203.0.113.7", Status: 1, X25519PublicKey: "a2V5"},
		},
	}
}

func namesOf(list []proton.LogicalServer) []string {
	out := make([]string, 0, len(list))
	for i := range list {
		out = append(out, list[i].Name)
	}
	return out
}
