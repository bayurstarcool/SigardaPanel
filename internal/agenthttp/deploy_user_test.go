package agenthttp

import "os"

// useCurrentDeployUser makes deploys run as the test process's own account
// (tests do not create Linux users); it returns the restore func.
func useCurrentDeployUser() func() {
	prev, prevSelf := deployUserIDs, deployAsSelf
	deployAsSelf = true
	deployUserIDs = func(string) (uint32, uint32, bool) {
		return uint32(os.Getuid()), uint32(os.Getgid()), true
	}
	return func() { deployUserIDs, deployAsSelf = prev, prevSelf }
}
