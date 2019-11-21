/*
Copyright 2019 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package label

import (
	"crypto/sha256"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation"
)

// GetValidName converts an input string to valid kubernetes label string in accordance to rfc1035 DNS Label spec
// (https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/identifiers.md)
// Length of the label is adjusted basis the DNS1035LabelMaxLength (defined at k8s.io/apimachinery/pkg/util/validation)
// If length exceeds, we trim the label name to contain only max allowed characters
// Additionally, the last 6 characters of the label name are replaced by the first 6 characters of the sha256 of original label
func GetValidName(label string) string {
	if len(label) <= validation.DNS1035LabelMaxLength {
		return label
	}

	sha := sha256.Sum256([]byte(label))
	strSha := fmt.Sprintf("%x", sha)
	charsFromLabel := validation.DNS1035LabelMaxLength - 6
	if charsFromLabel < 0 {
		// Derive the label name from sha hash in case the DNS1035LabelMaxLength is less than 6
		return string(strSha[validation.DNS1035LabelMaxLength])
	}

	return label[:charsFromLabel] + strSha[:6]
}
