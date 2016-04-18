package dchan

import (
	"bytes"
	"net"
	"time"

	"github.com/chzyer/flow"
	"github.com/chzyer/next/packet"
	"gopkg.in/logex.v1"
)

var (
	ErrInvalidUserId        = logex.Define("invalid user id")
	ErrUnexpectedPacketType = logex.Define("unexpected packet type")
)

var _ ChannelFactory = TcpChanFactory{}

type TcpChanFactory struct{}

func (TcpChanFactory) New(f *flow.Flow, session *packet.SessionIV, conn net.Conn, out chan<- *packet.Packet) Channel {
	return NewTcpChan(f, session, conn, out)
}

// try resend or timeout
func (TcpChanFactory) CliAuth(conn net.Conn, session *packet.SessionIV) error {
	p := packet.New(session.Token, packet.AUTH)
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(p.Marshal(session)); err != nil {
		return logex.Trace(err)
	}
	conn.SetWriteDeadline(time.Time{})

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	pr, err := packet.Read(session, conn)
	if err != nil {
		return logex.Trace(err)
	}
	conn.SetReadDeadline(time.Time{})

	if !bytes.Equal(pr.Payload, p.Payload) {
		return logex.NewError("invalid auth reply", pr.Payload, p.Payload)
	}
	return nil
}

func (TcpChanFactory) SvrAuth(delegate SvrAuthDelegate, conn net.Conn, port int) (*packet.SessionIV, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	iv, err := packet.ReadIV(conn)
	if err != nil {
		return nil, logex.Trace(err)
	}
	conn.SetReadDeadline(time.Time{})
	if int(iv.Port) != port {
		return nil, packet.ErrPortNotMatch.Trace()
	}

	token := delegate.GetUserToken(int(iv.UserId))
	if token == "" {
		return nil, ErrInvalidUserId.Trace()
	}

	s := packet.NewSessionIV(iv.UserId, iv.Port, []byte(token))
	p, err := packet.ReadWithIV(s, iv, conn)
	if err != nil {
		return nil, logex.Trace(err)
	}
	if p.Type != packet.AUTH {
		return nil, ErrUnexpectedPacketType.Trace()
	}
	if !bytes.Equal(s.Token, p.Payload) {
		return nil, packet.ErrInvalidToken.Trace()
	}

	p = packet.New(s.Token, packet.AUTH_R)
	if _, err := conn.Write(p.Marshal(s)); err != nil {
		return nil, logex.Trace(err)
	}
	return s, nil
}