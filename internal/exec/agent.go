package exec

import (
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// agentClientSigners reads whatever keys a running ssh-agent holds.
//
// CTP asked jsch for publickey as its second preference. Honouring that through
// the agent means an operator who already has keys loaded does not have to put a
// password in a configuration file to reach a machine.
func agentClientSigners(conn net.Conn) ([]ssh.Signer, error) {
	return agent.NewClient(conn).Signers()
}
