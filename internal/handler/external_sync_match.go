package handler

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"miaomiaowu/internal/storage"
)

// subInfoSuffixRE 匹配节点名末尾的流量/到期后缀，例如 " 12.34GB📊 5Days⏳"。
var subInfoSuffixRE = regexp.MustCompile(`(?:\s+\d+(?:\.\d+)?(?:KB|MB|GB|TB)📊)(?:\s+-?\d+Days⏳)?$|\s+-?\d+Days⏳$`)

type externalSyncStats struct {
	Updated int
	Created int
	Deleted int
	Skipped int
}

func (s externalSyncStats) Applied() int {
	return s.Updated + s.Created
}

func (s externalSyncStats) Summary() string {
	return fmt.Sprintf("更新 %d 个、新增 %d 个、删除 %d 个", s.Updated, s.Created, s.Deleted)
}

func (s *externalSyncStats) add(other externalSyncStats) {
	s.Updated += other.Updated
	s.Created += other.Created
	s.Deleted += other.Deleted
	s.Skipped += other.Skipped
}

func stripSubInfoSuffix(name string) string {
	return strings.TrimSpace(subInfoSuffixRE.ReplaceAllString(name, ""))
}

// nodeNameForMatching 去掉流量后缀和展示前缀，得到用于 node_name 匹配的机场原始名。
func nodeNameForMatching(nodeName, appliedPrefix, desiredPrefix string) string {
	name := stripSubInfoSuffix(nodeName)
	stripped := nodeNameWithoutPrefix(desiredPrefix, name)
	if stripped != name {
		return stripped
	}
	return nodeNameWithoutPrefix(appliedPrefix, name)
}

func nodeBelongsToExternalSubscription(node storage.Node, sub storage.ExternalSubscription, activeURLs map[string]bool) bool {
	return shouldPruneExternalSyncNode(node, sub, activeURLs)
}

func shouldRefreshSubscriptionTags(node storage.Node, subName string) bool {
	subName = strings.TrimSpace(subName)
	tag := strings.TrimSpace(node.Tag)
	tags := make([]string, 0, len(node.Tags))
	for _, item := range node.Tags {
		item = strings.TrimSpace(item)
		if item != "" {
			tags = append(tags, item)
		}
	}
	if tag == "" && len(tags) == 0 {
		return true
	}
	if tag != "" && tag != subName {
		return false
	}
	for _, item := range tags {
		if item != subName {
			return false
		}
	}
	return true
}

func incomingIdentityKey(rule string, clash map[string]any, remoteOriginal string) string {
	server, _ := clash["server"].(string)
	port := fmt.Sprintf("%v", clash["port"])
	typ, _ := clash["type"].(string)
	switch rule {
	case "server_port":
		return "sp:" + server + ":" + port
	case "type_server_port":
		return "tsp:" + typ + ":" + server + ":" + port
	case "type_server_port_cred":
		return "tspc:" + typ + ":" + server + ":" + port + "#" + nodeCredentialKey(clash)
	default:
		return "name:" + remoteOriginal
	}
}

func existingServerPortType(node storage.Node) (server string, port string, typ string, cred string, ok bool) {
	var clash map[string]any
	if err := json.Unmarshal([]byte(node.ClashConfig), &clash); err != nil {
		return "", "", "", "", false
	}
	server, _ = clash["server"].(string)
	if node.OriginalServer != "" {
		server = node.OriginalServer
	}
	port = fmt.Sprintf("%v", clash["port"])
	typ, _ = clash["type"].(string)
	return server, port, typ, nodeCredentialKey(clash), true
}

func matchByServerPort(existing []storage.Node, claimed map[int64]bool, newServer string, newPort any, requireType, requireCred bool, newType, newCred string) *storage.Node {
	if newServer == "" || newPort == nil || (requireType && newType == "") {
		return nil
	}
	wantPort := fmt.Sprintf("%v", newPort)
	for i := range existing {
		if claimed[existing[i].ID] {
			continue
		}
		server, port, typ, cred, ok := existingServerPortType(existing[i])
		if !ok || server != newServer || port != wantPort {
			continue
		}
		if requireType && typ != newType {
			continue
		}
		if requireCred && cred != newCred {
			continue
		}
		return &existing[i]
	}
	return nil
}

// matchExternalNodeByName 只在传入的（已限定为本订阅、且未占用）节点里按原始名匹配。
// 比较前会去掉流量/到期后缀和展示前缀。同一原始名有多条时保留最小 ID，其余当重复删除。
func matchExternalNodeByName(existing []storage.Node, claimed map[int64]bool, displayName, remoteOriginal, appliedPrefix, desiredPrefix string) (*storage.Node, []int64) {
	candidateIdx := make([]int, 0, 2)
	for i := range existing {
		if claimed[existing[i].ID] {
			continue
		}
		name := existing[i].NodeName
		if name == displayName ||
			nodeNameForMatching(name, appliedPrefix, desiredPrefix) == remoteOriginal ||
			sameRemoteNodeName(stripSubInfoSuffix(name), remoteOriginal, appliedPrefix, desiredPrefix) {
			candidateIdx = append(candidateIdx, i)
		}
	}
	if len(candidateIdx) == 0 {
		return nil, nil
	}
	best := candidateIdx[0]
	for _, i := range candidateIdx[1:] {
		if existing[i].ID < existing[best].ID {
			best = i
		}
	}
	dupIDs := make([]int64, 0, len(candidateIdx)-1)
	for _, i := range candidateIdx {
		if i != best && existing[i].ID != 0 {
			dupIDs = append(dupIDs, existing[i].ID)
		}
	}
	return &existing[best], dupIDs
}

func matchIncomingExternalNode(
	rule string,
	existing []storage.Node,
	claimed map[int64]bool,
	incomingClash map[string]any,
	displayName, remoteOriginal, appliedPrefix, desiredPrefix string,
) (*storage.Node, []int64) {
	newServer, _ := incomingClash["server"].(string)
	newPort := incomingClash["port"]
	newType, _ := incomingClash["type"].(string)
	switch rule {
	case "server_port":
		return matchByServerPort(existing, claimed, newServer, newPort, false, false, "", ""), nil
	case "type_server_port":
		return matchByServerPort(existing, claimed, newServer, newPort, true, false, newType, ""), nil
	case "type_server_port_cred":
		return matchByServerPort(existing, claimed, newServer, newPort, true, true, newType, nodeCredentialKey(incomingClash)), nil
	default:
		node, dups := matchExternalNodeByName(existing, claimed, displayName, remoteOriginal, appliedPrefix, desiredPrefix)
		if node != nil {
			return node, dups
		}
		// 自定义名或机场改名时，用本订阅内 type+server+port+凭据再认一次，避免删掉重建。
		return matchByServerPort(existing, claimed, newServer, newPort, true, true, newType, nodeCredentialKey(incomingClash)), nil
	}
}
