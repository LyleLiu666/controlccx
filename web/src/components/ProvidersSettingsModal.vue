<script setup lang="ts">
import { computed, ref } from "vue";
import type {
  AuthStatus,
  ProviderActiveSelection,
  ProviderProfile,
  ProviderSpeedTestResult,
} from "../types";

type SecretaryBackend = "auto" | "simple-http" | "claude" | "codex";
type SpeedTestTarget = "claude" | "codex" | "";
type ProviderTarget = "claude" | "codex" | "secretary";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  saving: boolean;
  error: string;
  storagePath: string;
  authStatus: AuthStatus | null;
  profiles: ProviderProfile[];
  active: ProviderActiveSelection;

  editID: string;
  editName: string;

  claudeBaseURL: string;
  claudeApiKey: string;
  claudeAuthToken: string;
  claudeModel: string;
  claudeSmallFastModel: string;
  claudeSyncLive: boolean;
  claudeApiKeyHint: string;
  claudeAuthTokenHint: string;

  codexBaseURL: string;
  codexApiKey: string;
  codexModel: string;
  codexReasoningEffort: string;
  codexSyncLive: boolean;
  codexApiKeyHint: string;

  secretaryBackend: SecretaryBackend;
  secretarySimpleHTTPBaseURL: string;
  secretarySimpleHTTPApiKey: string;
  secretarySimpleHTTPAuthToken: string;
  secretarySimpleHTTPModel: string;
  secretarySimpleHTTPApiKeyHint: string;
  secretarySimpleHTTPAuthTokenHint: string;
  secretarySyncLive: boolean;

  speedTesting: boolean;
  speedTestTarget: SpeedTestTarget;
  claudeSpeedTest: ProviderSpeedTestResult | null;
  codexSpeedTest: ProviderSpeedTestResult | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "newProfile"): void;
  (e: "refresh"): void;
  (e: "importLive"): void;
  (e: "export", includeSecrets: boolean): void;
  (e: "speedtest", target: "claude" | "codex"): void;
  (e: "selectProfile", profile: ProviderProfile): void;
  (e: "delete"): void;
  (e: "save"): void;
  (e: "activate", target: "claude" | "codex" | "secretary"): void;

  (e: "update:editID", value: string): void;
  (e: "update:editName", value: string): void;
  (e: "update:claudeBaseURL", value: string): void;
  (e: "update:claudeApiKey", value: string): void;
  (e: "update:claudeAuthToken", value: string): void;
  (e: "update:claudeModel", value: string): void;
  (e: "update:claudeSmallFastModel", value: string): void;
  (e: "update:claudeSyncLive", value: boolean): void;

  (e: "update:codexBaseURL", value: string): void;
  (e: "update:codexApiKey", value: string): void;
  (e: "update:codexModel", value: string): void;
  (e: "update:codexReasoningEffort", value: string): void;
  (e: "update:codexSyncLive", value: boolean): void;

  (e: "update:secretaryBackend", value: SecretaryBackend): void;
  (e: "update:secretarySimpleHTTPBaseURL", value: string): void;
  (e: "update:secretarySimpleHTTPApiKey", value: string): void;
  (e: "update:secretarySimpleHTTPAuthToken", value: string): void;
  (e: "update:secretarySimpleHTTPModel", value: string): void;
  (e: "update:secretarySyncLive", value: boolean): void;
}>();

const editIDModel = computed({
  get: () => props.editID,
  set: (value: string) => emit("update:editID", value),
});
const editNameModel = computed({
  get: () => props.editName,
  set: (value: string) => emit("update:editName", value),
});

const claudeBaseURLModel = computed({
  get: () => props.claudeBaseURL,
  set: (value: string) => emit("update:claudeBaseURL", value),
});
const claudeApiKeyModel = computed({
  get: () => props.claudeApiKey,
  set: (value: string) => emit("update:claudeApiKey", value),
});
const claudeAuthTokenModel = computed({
  get: () => props.claudeAuthToken,
  set: (value: string) => emit("update:claudeAuthToken", value),
});
const claudeModelModel = computed({
  get: () => props.claudeModel,
  set: (value: string) => emit("update:claudeModel", value),
});
const claudeSmallFastModelModel = computed({
  get: () => props.claudeSmallFastModel,
  set: (value: string) => emit("update:claudeSmallFastModel", value),
});
const claudeSyncLiveModel = computed({
  get: () => props.claudeSyncLive,
  set: (value: boolean) => emit("update:claudeSyncLive", value),
});

const codexBaseURLModel = computed({
  get: () => props.codexBaseURL,
  set: (value: string) => emit("update:codexBaseURL", value),
});
const codexApiKeyModel = computed({
  get: () => props.codexApiKey,
  set: (value: string) => emit("update:codexApiKey", value),
});
const codexModelModel = computed({
  get: () => props.codexModel,
  set: (value: string) => emit("update:codexModel", value),
});
const codexReasoningEffortModel = computed({
  get: () => props.codexReasoningEffort,
  set: (value: string) => emit("update:codexReasoningEffort", value),
});
const codexSyncLiveModel = computed({
  get: () => props.codexSyncLive,
  set: (value: boolean) => emit("update:codexSyncLive", value),
});

const secretaryBackendModel = computed({
  get: () => props.secretaryBackend,
  set: (value: SecretaryBackend) => emit("update:secretaryBackend", value),
});
const secretarySimpleHTTPBaseURLModel = computed({
  get: () => props.secretarySimpleHTTPBaseURL,
  set: (value: string) => emit("update:secretarySimpleHTTPBaseURL", value),
});
const secretarySimpleHTTPApiKeyModel = computed({
  get: () => props.secretarySimpleHTTPApiKey,
  set: (value: string) => emit("update:secretarySimpleHTTPApiKey", value),
});
const secretarySimpleHTTPAuthTokenModel = computed({
  get: () => props.secretarySimpleHTTPAuthToken,
  set: (value: string) => emit("update:secretarySimpleHTTPAuthToken", value),
});
const secretarySimpleHTTPModelModel = computed({
  get: () => props.secretarySimpleHTTPModel,
  set: (value: string) => emit("update:secretarySimpleHTTPModel", value),
});
const secretarySyncLiveModel = computed({
  get: () => props.secretarySyncLive,
  set: (value: boolean) => emit("update:secretarySyncLive", value),
});

const editorTab = ref<ProviderTarget>("claude");

function activateProfileForTarget(profile: ProviderProfile, target: ProviderTarget) {
  editorTab.value = target;
  emit("selectProfile", profile);
  emit("activate", target);
}
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal toolsModal providersModal">
      <div class="modalHeader providersHeader">
          <div class="providersHeaderLead">
            <div class="modalTitle">Providers</div>
          <div class="providersHeaderHint">Manage Claude Code, Codex, and Secretary profiles.</div>
          </div>
        <div class="providersHeaderActions">
          <button type="button" class="headerMiniBtn" @click="emit('newProfile')">
            New
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('refresh')"
            :disabled="loading || saving"
          >
            Refresh
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('importLive')"
            :disabled="loading || saving"
          >
            Import live
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('export', false)"
            :disabled="loading || saving"
          >
            Export
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('export', true)"
            :disabled="loading || saving"
          >
            Export secrets
          </button>
        </div>
        <button class="iconBtn providersCloseBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody toolsBody providersBody">
        <div
          v-if="storagePath || authStatus?.warnings?.length"
          class="providersMeta"
        >
          <div v-if="storagePath" class="settingsMeta providersStorage">
            Storage: <span class="mono">{{ storagePath }}</span>
          </div>
          <div v-if="authStatus?.warnings?.length" class="providersWarning">
            <div class="providersWarningTitle">Env overrides detected</div>
            <div class="tinyHint">
              Unset these environment variables and restart ControlCCX to make provider
              switches take effect.
            </div>
            <div class="providersWarningList mono">
              <div v-for="w in authStatus.warnings" :key="w">{{ w }}</div>
            </div>
          </div>
        </div>
        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading providersLoading">Loading...</div>
        <template v-else>
          <div class="toolsSplit providersSplit">
            <div class="toolsList providersList">
              <div class="providersListTitleRow">
                <div class="tinyHint">Profiles</div>
                <div class="tinyHint providersListLegend">
                  Activate per target: Claude Code / Codex / Secretary
                </div>
              </div>

              <div v-if="!profiles.length" class="tinyHint providersEmpty">
                No provider profiles yet. Click <span class="mono">New</span> to add one.
              </div>

              <div
                v-for="p in profiles"
                :key="p.id"
                class="providersItem"
                :class="{ active: p.id === editID }"
              >
                <button
                  type="button"
                  class="providersItemSelect"
                  @click="emit('selectProfile', p)"
                >
                  <div class="providersItemHead">
                    <div class="mono providersItemName">{{ p.name || p.id }}</div>
                    <span v-if="p.id === editID" class="providersItemTag">editing</span>
                  </div>
                  <div class="tinyHint mono providersItemID">{{ p.id }}</div>
                </button>

                <div class="providersTargets">
                  <button
                    type="button"
                    class="providersTarget"
                    :class="{ on: active.claude === p.id }"
                    :disabled="loading || saving"
                    title="Activate this profile for Claude Code"
                    @click="activateProfileForTarget(p, 'claude')"
                  >
                    Claude Code
                  </button>
                  <button
                    type="button"
                    class="providersTarget"
                    :class="{ on: active.codex === p.id }"
                    :disabled="loading || saving"
                    title="Activate this profile for Codex"
                    @click="activateProfileForTarget(p, 'codex')"
                  >
                    Codex
                  </button>
                  <button
                    type="button"
                    class="providersTarget"
                    :class="{ on: active.secretary === p.id }"
                    :disabled="loading || saving"
                    title="Activate this profile for Secretary"
                    @click="activateProfileForTarget(p, 'secretary')"
                  >
                    Secretary
                  </button>
                </div>
              </div>
            </div>

            <div class="toolsEditor providersEditor">
              <div class="toolsEditorGrid">
                <label class="full">
                  name
                  <input
                    v-model="editNameModel"
                    placeholder="Current"
                    autocomplete="off"
                  />
                </label>

                <div class="providersTabs" role="tablist" aria-label="Provider target">
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'claude' }"
                    @click="editorTab = 'claude'"
                  >
                    Claude Code
                  </button>
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'codex' }"
                    @click="editorTab = 'codex'"
                  >
                    Codex
                  </button>
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'secretary' }"
                    @click="editorTab = 'secretary'"
                  >
                    Secretary
                  </button>
                </div>

                <div v-if="editorTab === 'claude'" class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <label class="full">
                      base url
                      <input
                        v-model="claudeBaseURLModel"
                        placeholder="https://api.anthropic.com"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      auth token (preferred)
                      <input
                        v-model="claudeAuthTokenModel"
                        type="password"
                        :placeholder="
                          claudeAuthTokenHint
                            ? `Leave blank to keep (${claudeAuthTokenHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      api key
                      <input
                        v-model="claudeApiKeyModel"
                        type="password"
                        :placeholder="
                          claudeApiKeyHint
                            ? `Leave blank to keep (${claudeApiKeyHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      model
                      <input
                        v-model="claudeModelModel"
                        placeholder="claude-3-7-sonnet"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      small fast model
                      <input
                        v-model="claudeSmallFastModelModel"
                        placeholder="claude-3-5-haiku"
                        autocomplete="off"
                      />
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="claudeSyncLiveModel" />
                      <span>Sync live config on activate</span>
                    </label>
                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'claude')"
                        :disabled="
                          saving ||
                          !editID.trim() ||
                          speedTesting
                        "
                      >
                        <span
                          v-if="speedTesting && speedTestTarget === 'claude'"
                          >Testing...</span
                        >
                        <span v-else>Speed test</span>
                      </button>
                      <div
                        v-if="claudeSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: claudeSpeedTest.ok, bad: !claudeSpeedTest.ok }"
                      >
                        <span>{{ claudeSpeedTest.ok ? "ok" : "fail" }}</span>
                        <span v-if="claudeSpeedTest.latency_ms != null">
                          {{ claudeSpeedTest.latency_ms }}ms</span
                        >
                        <span
                          v-if="
                            !claudeSpeedTest.ok &&
                            (claudeSpeedTest.hint || claudeSpeedTest.error)
                          "
                        >
                          {{ claudeSpeedTest.hint || claudeSpeedTest.error }}</span
                        >
                      </div>
                    </div>
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'claude')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Claude Code
                      </button>
                    </div>
                  </div>
                </div>

                <div v-else-if="editorTab === 'codex'" class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <label class="full">
                      base url (optional)
                      <input
                        v-model="codexBaseURLModel"
                        placeholder="https://api.openai.com"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      api key
                      <input
                        v-model="codexApiKeyModel"
                        type="password"
                        :placeholder="
                          codexApiKeyHint
                            ? `Leave blank to keep (${codexApiKeyHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      model
                      <input
                        v-model="codexModelModel"
                        placeholder="gpt-5.2"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      reasoning effort
                      <select v-model="codexReasoningEffortModel">
                        <option value="">(default)</option>
                        <option value="low">low</option>
                        <option value="medium">medium</option>
                        <option value="high">high</option>
                        <option value="xhigh">xhigh</option>
                      </select>
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="codexSyncLiveModel" />
                      <span>Sync live config on activate</span>
                    </label>
                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'codex')"
                        :disabled="
                          saving ||
                          !editID.trim() ||
                          speedTesting
                        "
                      >
                        <span
                          v-if="speedTesting && speedTestTarget === 'codex'"
                          >Testing...</span
                        >
                        <span v-else>Speed test</span>
                      </button>
                      <div
                        v-if="codexSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: codexSpeedTest.ok, bad: !codexSpeedTest.ok }"
                      >
                        <span>{{ codexSpeedTest.ok ? "ok" : "fail" }}</span>
                        <span v-if="codexSpeedTest.latency_ms != null">
                          {{ codexSpeedTest.latency_ms }}ms</span
                        >
                        <span
                          v-if="
                            !codexSpeedTest.ok &&
                            (codexSpeedTest.hint || codexSpeedTest.error)
                          "
                        >
                          {{ codexSpeedTest.hint || codexSpeedTest.error }}</span
                        >
                      </div>
                    </div>
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'codex')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Codex
                      </button>
                    </div>
                  </div>
                </div>

                <div v-else class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <label class="full">
                      backend
                      <select v-model="secretaryBackendModel">
                        <option value="auto">auto</option>
                        <option value="simple-http">simple-http</option>
                        <option value="claude">claude</option>
                        <option value="codex">codex</option>
                      </select>
                    </label>

                    <div
                      v-if="secretaryBackendModel === 'auto' || secretaryBackendModel === 'simple-http'"
                      class="providersSubsection"
                    >
                      <div class="providersSubsectionTitle">Simple HTTP Auth (Anthropic)</div>
                      <div class="tinyHint">
                        Used when Secretary backend is <span class="mono">auto</span> and simple-http is selected, or
                        when backend is <span class="mono">simple-http</span>.
                      </div>
                      <div class="toolsEditorGrid providersSubsectionGrid">
                        <label class="full">
                          base url
                          <input
                            v-model="secretarySimpleHTTPBaseURLModel"
                            placeholder="https://api.anthropic.com"
                            autocomplete="off"
                          />
                        </label>
                        <label class="full">
                          auth token (preferred)
                          <input
                            v-model="secretarySimpleHTTPAuthTokenModel"
                            type="password"
                            :placeholder="
                              secretarySimpleHTTPAuthTokenHint
                                ? `Leave blank to keep (${secretarySimpleHTTPAuthTokenHint})`
                                : 'Leave blank to keep'
                            "
                            autocomplete="off"
                          />
                        </label>
                        <label class="full">
                          api key
                          <input
                            v-model="secretarySimpleHTTPApiKeyModel"
                            type="password"
                            :placeholder="
                              secretarySimpleHTTPApiKeyHint
                                ? `Leave blank to keep (${secretarySimpleHTTPApiKeyHint})`
                                : 'Leave blank to keep'
                            "
                            autocomplete="off"
                          />
                        </label>
                        <label class="full">
                          model
                          <input
                            v-model="secretarySimpleHTTPModelModel"
                            placeholder="claude-3-5-sonnet-latest"
                            autocomplete="off"
                          />
                        </label>
                      </div>
                    </div>

                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="secretarySyncLiveModel" disabled />
                      <span>Sync live config on activate (n/a)</span>
                    </label>
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'secretary')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Secretary
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">Close</button>
        <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">
          Delete
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving || !editName.trim()"
        >
          {{ saving ? "Saving..." : "Save" }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.providersModal {
  width: min(1160px, 96vw);
  height: min(760px, 92vh);
  border-radius: 20px;
  background:
    radial-gradient(1100px 460px at 8% -14%, rgba(20, 184, 166, 0.09), transparent 55%),
    radial-gradient(760px 380px at 100% -30%, rgba(56, 189, 248, 0.1), transparent 56%),
    var(--bg-panel);
}

.providersHeader {
  align-items: flex-start;
  gap: 12px;
  border-bottom-color: color-mix(in srgb, var(--border-color) 78%, transparent);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--bg-subtle) 92%, white 8%), var(--bg-subtle));
}

.providersHeaderLead {
  display: grid;
  gap: 4px;
  min-width: 220px;
}

.providersHeaderHint {
  font-size: 12px;
  color: var(--text-sub);
  letter-spacing: 0.01em;
}

.providersHeaderActions {
  margin-left: auto;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.providersHeaderActions .headerMiniBtn {
  min-height: 36px;
  padding: 8px 14px;
  border-radius: 999px;
  border-color: color-mix(in srgb, var(--border-color) 80%, rgba(20, 184, 166, 0.35) 20%);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
}

.providersHeaderActions .headerMiniBtn:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--color-primary) 36%, var(--border-color));
  background: color-mix(in srgb, var(--color-primary-bg) 40%, var(--bg-panel));
}

.providersCloseBtn {
  margin-left: 2px;
}

.providersBody {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow: hidden;
}

.providersMeta {
  display: grid;
  gap: 10px;
}

.providersStorage {
  margin: 0;
  border: 1px dashed color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.28) 24%);
  border-radius: 12px;
  background: color-mix(in srgb, var(--bg-subtle) 92%, white 8%);
  padding: 10px 12px;
  color: var(--text-sub);
}

.providersLoading {
  display: grid;
  place-items: center;
  border: 1px dashed var(--border-color);
  border-radius: 14px;
  min-height: 160px;
  color: var(--text-sub);
}

.providersSplit {
  grid-template-columns: minmax(240px, 300px) minmax(0, 1fr);
  flex: 1;
  min-height: 0;
}

.providersList {
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--border-color) 85%, rgba(20, 184, 166, 0.2) 15%);
  background: color-mix(in srgb, var(--bg-subtle) 93%, white 7%);
  padding: 10px;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.providersListTitleRow {
  display: grid;
  gap: 4px;
  padding: 2px 2px 6px;
}

.providersListLegend {
  color: var(--text-sub);
}

.providersEmpty {
  padding: 8px 2px;
}

.providersItem {
  border: 1px solid transparent;
  border-radius: 12px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--bg-panel) 84%, transparent);
  display: grid;
  gap: 10px;
  transition: background-color 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease;
}

.providersItem:hover {
  border-color: color-mix(in srgb, var(--border-color) 60%, rgba(20, 184, 166, 0.38) 40%);
  background: color-mix(in srgb, var(--bg-subtle) 58%, var(--bg-panel));
}

.providersItem.active {
  border-color: color-mix(in srgb, var(--color-primary) 48%, transparent);
  background:
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--color-primary-bg) 56%, transparent),
      color-mix(in srgb, var(--bg-panel) 80%, transparent)
    );
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.providersItemSelect {
  width: 100%;
  border: none;
  background: transparent;
  padding: 0;
  text-align: left;
  cursor: pointer;
  display: grid;
  gap: 6px;
}

.providersItemSelect:disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.providersItemHead {
  display: flex;
  align-items: center;
  gap: 8px;
}

.providersItemName {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.providersItemID {
  color: var(--text-sub);
}

.providersItemTag {
  margin-left: auto;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--color-primary) 32%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 55%, transparent);
  color: var(--text-main);
  font-size: 10px;
  font-weight: 800;
  line-height: 1;
  padding: 4px 7px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.providersWarning {
  border: 1px solid color-mix(in srgb, rgba(245, 158, 11, 0.6) 62%, var(--border-color));
  background: color-mix(in srgb, rgba(245, 158, 11, 0.14) 82%, var(--bg-panel));
  border-radius: 14px;
  padding: 12px 14px;
  margin: 0;
}

.providersWarningTitle {
  font-weight: 900;
  color: var(--text-main);
  margin-bottom: 6px;
}

.providersWarningList {
  margin-top: 8px;
  opacity: 0.95;
}

.providersTargets {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.providersTarget {
  min-height: 32px;
  padding: 6px 10px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--border-color) 80%, rgba(20, 184, 166, 0.2) 20%);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  color: var(--text-sub);
  font-weight: 800;
  font-size: 11px;
  letter-spacing: 0.02em;
}

.providersTarget:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--color-primary) 40%, var(--border-color));
  background: color-mix(in srgb, var(--color-primary-bg) 44%, var(--bg-panel));
  color: var(--text-main);
}

.providersTarget:disabled {
  opacity: 0.65;
  cursor: not-allowed;
}

.providersTarget.on {
  border-color: color-mix(in srgb, var(--color-primary) 54%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 60%, transparent);
  color: var(--text-main);
}

.providersEditor {
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--border-color) 84%, rgba(20, 184, 166, 0.2) 16%);
  background: color-mix(in srgb, var(--bg-subtle) 93%, white 7%);
  padding: 14px;
  min-height: 0;
  overflow: auto;
}

.providersTabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px;
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 14px;
  background: color-mix(in srgb, var(--bg-panel) 78%, transparent);
}

.providersTab {
  flex: 1 1 120px;
  min-height: 34px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-sub);
  font-weight: 900;
}

.providersTab:hover:not(.on) {
  background: color-mix(in srgb, var(--bg-subtle) 70%, transparent);
  color: var(--text-main);
}

.providersTab.on {
  border-color: color-mix(in srgb, var(--color-primary) 46%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 56%, transparent);
  color: var(--text-main);
}

.providersTabPanel {
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 14px;
  padding: 12px;
  background: color-mix(in srgb, var(--bg-panel) 84%, transparent);
}

.providersSubsection {
  border: 1px dashed color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.28) 24%);
  border-radius: 14px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  display: grid;
  gap: 8px;
}

.providersSubsectionTitle {
  font-weight: 900;
  color: var(--text-main);
}

.providersSubsectionGrid {
  margin-top: 4px;
}

.providerActions {
  display: flex;
  justify-content: flex-end;
}

.providerTestRow {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.speedTestResult {
  padding: 4px 8px;
  border-radius: 999px;
  border: 1px solid var(--border-color);
  background: rgba(255, 255, 255, 0.04);
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.2;
}

.speedTestResult.ok {
  border-color: rgba(20, 184, 166, 0.45);
  background: rgba(20, 184, 166, 0.12);
  color: var(--text-main);
}

.speedTestResult.bad {
  border-color: rgba(248, 113, 113, 0.4);
  background: rgba(248, 113, 113, 0.08);
  color: var(--text-main);
}

.providersList,
.providersEditor {
  scrollbar-width: thin;
  scrollbar-color: color-mix(in srgb, var(--text-sub) 55%, transparent) transparent;
}

.providersList::-webkit-scrollbar,
.providersEditor::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

.providersList::-webkit-scrollbar-thumb,
.providersEditor::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--text-sub) 45%, transparent);
  border-radius: 999px;
  border: 2px solid transparent;
  background-clip: content-box;
}

.providersList::-webkit-scrollbar-track,
.providersEditor::-webkit-scrollbar-track {
  background: transparent;
}

@media (max-width: 980px) {
  .providersHeader {
    padding-bottom: 14px;
  }

  .providersHeaderActions {
    margin-left: 0;
    width: 100%;
    justify-content: flex-start;
  }

  .providersSplit {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(180px, 34%) minmax(0, 1fr);
  }
}

@media (max-width: 720px) {
  .providersModal {
    width: min(98vw, 1160px);
    height: min(94vh, 920px);
  }

  .providersHeaderActions .headerMiniBtn {
    flex: 1 1 calc(50% - 8px);
  }
}
</style>
