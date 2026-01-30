<script setup lang="ts">
import { computed } from "vue";
import type { Skill, SkillsListResponse } from "../types";

type SkillTarget = "cursor" | "claude_code" | "codex";
type SkillsSummary = {
  target: SkillTarget;
  status:
    | "missing"
    | "linked"
    | "broken"
    | "present"
    | "copied"
    | "conflict"
    | "external"
    | "partial";
  canEnable: boolean;
  canDisable: boolean;
  enabled: boolean;
  detail: string;
};

const props = defineProps<{
  loading: boolean;
  error: string;
  data: SkillsListResponse | null;
  filter: string;
  limit: number;
  rangeLabel: string;
  canPrev: boolean;
  canNext: boolean;
  actionBusy: Map<string, boolean>;
  summarizeTarget: (skill: Skill, target: SkillTarget) => SkillsSummary;
  badgeClass: (status: string) => string;
  makeKey: (name: string, target: SkillTarget) => string;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "prevPage"): void;
  (e: "nextPage"): void;
  (e: "toggle", name: string, target: SkillTarget, enable: boolean): void;
  (e: "update:filter", value: string): void;
  (e: "update:limit", value: number): void;
}>();

const filterModel = computed({
  get: () => props.filter,
  set: (value: string) => emit("update:filter", value),
});
const limitModel = computed({
  get: () => props.limit,
  set: (value: number) => emit("update:limit", value),
});
const skillsVisible = computed(() => props.data?.skills ?? []);
</script>

<template>
  <div class="skillsBody skillsPageBody skillsPanelBody">
    <div v-if="error" class="modalError">{{ error }}</div>
    <div v-else-if="loading" class="loading">Loading...</div>
    <template v-else>
      <div class="skillsMeta">
        <div class="tinyHint">
          Sources:
          <span class="mono">{{ (data?.source_roots ?? []).join(" · ") }}</span>
        </div>
        <div class="tinyHint">
          Targets:
          <span class="mono">{{
            (data?.targets ?? []).map((t) => `${t.target}:${t.root}`).join(" · ")
          }}</span>
        </div>
      </div>

      <div class="skillsToolbar">
        <input v-model="filterModel" placeholder="Filter skills..." />
        <label class="skillsLimit">
          <span class="tinyHint">Page</span>
          <select v-model.number="limitModel" :disabled="loading">
            <option :value="50">50</option>
            <option :value="100">100</option>
            <option :value="200">200</option>
            <option :value="500">500</option>
          </select>
        </label>
        <div class="skillsPager">
          <span class="tinyHint mono">{{ rangeLabel }}</span>
          <button
            type="button"
            @click="emit('prevPage')"
            :disabled="loading || !canPrev"
            title="Previous page"
          >
            Prev
          </button>
          <button
            type="button"
            @click="emit('nextPage')"
            :disabled="loading || !canNext"
            title="Next page"
          >
            Next
          </button>
        </div>
      </div>

      <div class="skillsTable">
        <div class="skillsHead">
          <div>Skill</div>
          <div>Cursor</div>
          <div>Claude Code</div>
          <div>Codex</div>
        </div>

        <div class="skillsRows">
          <div v-for="s in skillsVisible" :key="s.name" class="skillsRow">
            <div class="skillsName">
              <div class="mono">{{ s.name }}</div>
              <div class="tinyHint mono" v-if="s.source" :title="s.source">
                {{ s.source }}
              </div>
              <div class="tinyHint warn" v-else>missing source</div>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'cursor')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ t.status.toUpperCase() }}</span
                >
                <button
                  type="button"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'cursor', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'cursor'))"
                  :title="
                    t.canEnable
                      ? 'Enable for Cursor'
                      : 'Cannot enable: unmanaged entry exists in a Cursor root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "cursor")) ? "..." : "Enable" }}
                </button>
                <button
                  type="button"
                  v-else
                  @click="emit('toggle', s.name, 'cursor', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'cursor'))"
                  :title="
                    t.canDisable
                      ? 'Disable for Cursor'
                      : 'Cannot disable: unmanaged entry exists in a Cursor root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "cursor")) ? "..." : "Disable" }}
                </button>
              </template>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'claude_code')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ t.status.toUpperCase() }}</span
                >
                <button
                  type="button"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'claude_code', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'claude_code'))"
                  :title="
                    t.canEnable
                      ? 'Enable for Claude Code'
                      : 'Cannot enable: unmanaged entry exists in a Claude Code root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "claude_code")) ? "..." : "Enable" }}
                </button>
                <button
                  type="button"
                  v-else
                  @click="emit('toggle', s.name, 'claude_code', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'claude_code'))"
                  :title="
                    t.canDisable
                      ? 'Disable for Claude Code'
                      : 'Cannot disable: unmanaged entry exists in a Claude Code root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "claude_code")) ? "..." : "Disable" }}
                </button>
              </template>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'codex')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ t.status.toUpperCase() }}</span
                >
                <button
                  type="button"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'codex', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'codex'))"
                  :title="
                    t.canEnable
                      ? 'Enable for Codex'
                      : 'Cannot enable: unmanaged entry exists in a Codex root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "codex")) ? "..." : "Enable" }}
                </button>
                <button
                  type="button"
                  v-else
                  @click="emit('toggle', s.name, 'codex', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'codex'))"
                  :title="
                    t.canDisable
                      ? 'Disable for Codex'
                      : 'Cannot disable: unmanaged entry exists in a Codex root'
                  "
                >
                  {{ actionBusy.get(makeKey(s.name, "codex")) ? "..." : "Disable" }}
                </button>
              </template>
            </div>
          </div>

          <div v-if="!skillsVisible.length" class="empty">No skills</div>
        </div>
      </div>
    </template>
  </div>
</template>
