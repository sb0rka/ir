package spec

import (
	"testing"
)

// Примеры в спеке — не украшение: на них стоит мок, из которого фронт собирает
// граф, пока ручки не написаны. Держатся они только на том, что идентификаторы
// расставлены согласованно вручную, и рассыпятся от первой же правки молча:
// линтер проверяет соответствие схеме, а не то, что ребро упирается
// в существующий узел.

const (
	graphPath    = "/investigations/{investigation_id}/graph"
	eventsPath   = "/investigations/{investigation_id}/events"
	entitiesPath = "/investigations/{investigation_id}/entities"
)

func TestGraphExampleIsConnected(t *testing.T) {
	graph := example(t, graphPath)

	nodes := list(t, graph, "nodes")
	edges := list(t, graph, "edges")
	if len(nodes) == 0 || len(edges) == 0 {
		t.Fatalf("пример графа пуст: %d узлов, %d рёбер", len(nodes), len(edges))
	}

	nodeIDs := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		nodeIDs[str(t, node, "id")] = true
	}

	for _, edge := range edges {
		for _, end := range []string{"source_node_id", "target_node_id"} {
			if id := str(t, edge, end); !nodeIDs[id] {
				t.Errorf("ребро %s: %s=%s не найден среди узлов", str(t, edge, "id"), end, id)
			}
		}
	}
}

// Все три статуса и все три origin — чтобы на моке можно было отрисовать
// каждое состояние ребра, а не только счастливый путь.
func TestGraphExampleCoversEveryEdgeState(t *testing.T) {
	edges := list(t, example(t, graphPath), "edges")

	for _, field := range []string{"status", "origin"} {
		seen := make(map[string]bool)
		for _, edge := range edges {
			seen[str(t, edge, field)] = true
		}
		want := map[string][]string{
			"status": {"proposed", "confirmed", "rejected"},
			"origin": {"analyst", "rule", "agent"},
		}[field]
		for _, value := range want {
			if !seen[value] {
				t.Errorf("в примере графа нет ребра с %s=%s", field, value)
			}
		}
	}
}

// Рёбра ссылаются на события таймлайна, узлы-сущности — на карточки сущностей.
// Разъедется — и на моке провенанс поедет в никуда.
func TestGraphExampleReferencesTimelineAndEntities(t *testing.T) {
	graph := example(t, graphPath)

	eventIDs := ids(t, list(t, example(t, eventsPath), "items"), "id")
	for _, edge := range list(t, graph, "edges") {
		for _, raw := range slice(edge["evidence_event_ids"]) {
			if id, _ := raw.(string); !eventIDs[id] {
				t.Errorf("ребро %s ссылается на событие %s, которого нет в примере таймлайна",
					str(t, edge, "id"), id)
			}
		}
	}

	entityIDs := ids(t, list(t, example(t, entitiesPath), "items"), "id")
	for _, node := range list(t, graph, "nodes") {
		if str(t, node, "node_type") != "entity" {
			continue
		}
		if id := str(t, node, "entity_id"); !entityIDs[id] {
			t.Errorf("узел %s ссылается на сущность %s, которой нет в примере списка",
				str(t, node, "id"), id)
		}
	}
}

// Узел обязан представлять ровно одно: сущность или событие. Инвариант держит
// CHECK в базе, но пример пишется руками и под него не попадает.
func TestGraphExampleNodesPointAtExactlyOneThing(t *testing.T) {
	for _, node := range list(t, example(t, graphPath), "nodes") {
		_, hasEntity := node["entity_id"]
		_, hasEvent := node["event_id"]

		switch str(t, node, "node_type") {
		case "entity":
			if !hasEntity || hasEvent {
				t.Errorf("узел %s: node_type=entity, но ссылки не те", str(t, node, "id"))
			}
		case "event":
			if !hasEvent || hasEntity {
				t.Errorf("узел %s: node_type=event, но ссылки не те", str(t, node, "id"))
			}
		default:
			t.Errorf("узел %s: неизвестный node_type", str(t, node, "id"))
		}
	}
}

func example(t *testing.T, path string) map[string]any {
	t.Helper()

	doc, err := GetSpec()
	if err != nil {
		t.Fatalf("загрузить спеку: %v", err)
	}
	item := doc.Paths.Find(path)
	if item == nil || item.Get == nil {
		t.Fatalf("в спеке нет GET %s", path)
	}
	response := item.Get.Responses.Status(200)
	if response == nil || response.Value == nil {
		t.Fatalf("GET %s: нет ответа 200", path)
	}
	media := response.Value.Content.Get("application/json")
	if media == nil || media.Example == nil {
		t.Fatalf("GET %s: нет примера — мок отдаст сгенерированный мусор", path)
	}
	value, ok := media.Example.(map[string]any)
	if !ok {
		t.Fatalf("GET %s: пример не объект", path)
	}
	return value
}

func list(t *testing.T, object map[string]any, field string) []map[string]any {
	t.Helper()

	out := make([]map[string]any, 0, len(slice(object[field])))
	for _, raw := range slice(object[field]) {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s: элемент не объект", field)
		}
		out = append(out, item)
	}
	return out
}

func slice(value any) []any {
	items, _ := value.([]any)
	return items
}

func str(t *testing.T, object map[string]any, field string) string {
	t.Helper()

	value, ok := object[field].(string)
	if !ok {
		t.Fatalf("поле %s отсутствует или не строка", field)
	}
	return value
}

func ids(t *testing.T, items []map[string]any, field string) map[string]bool {
	t.Helper()

	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[str(t, item, field)] = true
	}
	return out
}
