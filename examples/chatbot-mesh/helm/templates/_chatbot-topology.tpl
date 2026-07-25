{{/*
One source-count-independent topology declaration generated from ragUnits. The
classifier catalog exposes only names and descriptions; trusted items retain the
target/config fields used after select_subset. The program remains unchanged as
the list grows.
*/}}
{{- define "chatbot-mesh.chatbotTopology" -}}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
{{- $q := .Values.ragServer.ports.query -}}
tools:
  - name: declare_rag_topology
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Declare the ordered trusted RAG topology for this chatbot instance.
    problem: MachineSpec for_each needs one runtime array whose size may change without changing the program.
    goals:
      - Keep source identity and selected REST authority together.
      - Render a classifier catalog without exposing authority or configuration.
      - Preserve declaration order for sequential fan-out and reporting.
    requirements:
      input:
        - Topology is trusted profile configuration, not model or request input.
      output:
        - Output contains names, a human-readable catalog, and trusted items with source descriptions, configuration, and absolute base URLs.
      errors:
        - Rendering configured JSON is deterministic.
    non_goals:
      - Does not select a subset of sources or accept credentials.
    parameters: {type: object, properties: {}, additionalProperties: false}
    emits: [RagTopologyDeclared]
    output:
      description: Ordered RAG topology.
      schema:
        type: object
        properties:
          items: {type: array}
          names: {type: array}
          catalog: {type: string}
        required: [items, names, catalog]
    side_effects: []
    reversibility: {classification: reversible, undo: noop}
    undo: {strategy: noop, description: Declaring topology changes no external state.}
    config:
      signal: RagTopologyDeclared
      inputs: {}
      template: |
        {
          "names": [
{{- range $i, $unit := .Values.ragUnits }}
            {{ $unit.name | quote }}{{ if lt (add1 $i) (len $.Values.ragUnits) }},{{ end }}
{{- end }}
          ],
          "catalog": {{- $catalog := list -}}{{- range $unit := .Values.ragUnits -}}{{- $catalog = append $catalog (printf "- %s: %s" $unit.name $unit.description) -}}{{- end }} {{ join "\n" $catalog | quote }},
          "items": [
{{- range $i, $unit := .Values.ragUnits }}
            {
              "name": {{ $unit.name | quote }},
              "description": {{ $unit.description | quote }},
              "collection": {{ $unit.collection | quote }},
              "embedding_model": {{ $unit.embeddingModel | quote }},
              "base_url": {{ printf "http://%s-%s:%v" $fullname $unit.name $q | quote }}
            }{{ if lt (add1 $i) (len $.Values.ragUnits) }},{{ end }}
{{- end }}
          ]
        }
{{- end -}}
