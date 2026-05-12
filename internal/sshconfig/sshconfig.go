package sshconfig

import "golang.org/x/crypto/ssh"

func NewClientConfig(user string, auth []ssh.AuthMethod) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config: ssh.Config{
			KeyExchanges: []string{
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
			Ciphers: []string{
				"chacha20-poly1305@openssh.com",
				"aes256-ctr",
				"aes128-ctr",
				"aes256-cbc",
				"aes128-cbc",
				"3des-cbc",
			},
			MACs: []string{
				"hmac-sha2-256",
				"hmac-sha1",
			},
		},
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSA,
		},
	}
}
