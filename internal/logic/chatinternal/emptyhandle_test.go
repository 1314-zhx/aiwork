/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"testing"
)

func TestNewDefaultHandler(t *testing.T) {
	defaultHandler := NewDefaultHandler(svcTest)
	if defaultHandler.Chains() == nil {
		t.Fatal("expected default handler chain")
	}
}
