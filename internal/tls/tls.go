package tls

import (
	"context"
	"errors"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

var (
	ErrNoPeerInContext         = errors.New("no peer in context")
	ErrMissingTLSInfo          = errors.New("missing TLS info")
	ErrMissingPeerCertificates = errors.New("missing peer certificates")
)

// GetUserFromContext returns the user from the provided context by looking up the subject common name from the peer certificate.
func GetUserFromContext(ctx context.Context) (string, error) {
	peer, hasPeer := peer.FromContext(ctx)
	if !hasPeer {
		return "", ErrNoPeerInContext
	}

	tlsInfo, hasTLSInfo := peer.AuthInfo.(credentials.TLSInfo)
	if !hasTLSInfo {
		return "", ErrMissingTLSInfo
	}

	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", ErrMissingPeerCertificates
	}

	// assume only one peer certificate
	return tlsInfo.State.PeerCertificates[0].Subject.CommonName, nil
}
