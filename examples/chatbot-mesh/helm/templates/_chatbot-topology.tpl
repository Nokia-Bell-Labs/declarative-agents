{{/*
One source-count-independent topology declaration generated from ragUnits. The
list changes data only; request-fanout.yaml and request-machine.yaml are mounted
verbatim and retain one for_each regardless of source count.
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
      - Preserve declaration order for sequential fan-out and reporting.
    requirements:
      input:
        - Topology is trusted profile configuration, not model or request input.
      output:
        - Output contains an items array of source names and absolute base URLs.
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
        required: [items]
    side_effects: []
    reversibility: {classification: reversible, undo: noop}
    undo: {strategy: noop, description: Declaring topology changes no external state.}
    config:
      signal: RagTopologyDeclared
      inputs: {}
      template: |
        {
          "items": [
{{- range $i, $unit := .Values.ragUnits }}
            {"name": {{ $unit.name | quote }}, "base_url": {{ printf "http://%s-%s:%v" $fullname $unit.name $q | quote }}}{{ if lt (add1 $i) (len $.Values.ragUnits) }},{{ end }}
{{- end }}
          ]
        }
{{- end -}}
