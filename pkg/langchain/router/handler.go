/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package router

import "github.com/tmc/langchaingo/chains"

type Handler interface {
	Name() string
	Description() string
	Chains() chains.Chain
}
