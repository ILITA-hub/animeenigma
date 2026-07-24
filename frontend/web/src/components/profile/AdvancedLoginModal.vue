<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, Check } from 'lucide-vue-next'
import { Modal, Button, Input, Switch } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'
import { useAdvancedLogin, isPasskeySupported } from '@/composables/useAdvancedLogin'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()

const { t, locale } = useI18n()
const { confirm } = useConfirm()

const passkeySupported = isPasskeySupported()

const {
  passkeys,
  certs,
  caInfo,
  passkeysLoading,
  certsLoading,
  passkeyBusy,
  certBusy,
  autoLoginBusy,
  error,
  issuedPassword,
  hasActiveCert,
  certAutoLogin,
  refreshPasskeys,
  addPasskey,
  deletePasskey,
  refreshCerts,
  issueCert,
  revokeCert,
  setCertAutoLogin,
  clearIssuedPassword,
} = useAdvancedLogin()

const passkeyName = ref('')
const certName = ref('')
const passwordCopied = ref(false)

/** AA:BB:… presentation of a hex fingerprint (matches OS trust dialogs). */
function fmtFingerprint(hex: string): string {
  return hex.toUpperCase().match(/.{2}/g)?.join(':') ?? hex
}

function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(locale.value, { year: 'numeric', month: 'short', day: 'numeric' })
}

// Load data whenever the modal opens; reset transient state when it closes.
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      if (passkeySupported) refreshPasskeys()
      refreshCerts()
    } else {
      clearIssuedPassword()
      passwordCopied.value = false
      passkeyName.value = ''
      certName.value = ''
    }
  },
  { immediate: true },
)

function onModelUpdate(value: boolean): void {
  emit('update:open', value)
}

async function onAddPasskey(): Promise<void> {
  if (await addPasskey(passkeyName.value)) passkeyName.value = ''
}

async function onDeletePasskey(id: string): Promise<void> {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.advancedLogin.passkeys.confirmDelete'),
    confirmText: t('profile.advancedLogin.passkeys.delete'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  await deletePasskey(id)
}

async function onIssueCert(): Promise<void> {
  if (await issueCert(certName.value)) certName.value = ''
}

async function onRevokeCert(id: string): Promise<void> {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('profile.advancedLogin.cert.confirmRevoke'),
    confirmText: t('profile.advancedLogin.cert.revoke'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  await revokeCert(id)
}

async function copyPassword(): Promise<void> {
  if (!issuedPassword.value) return
  try {
    await navigator.clipboard.writeText(issuedPassword.value)
    passwordCopied.value = true
    setTimeout(() => { passwordCopied.value = false }, 2000)
  } catch { /* clipboard blocked — user can select manually */ }
}
</script>

<template>
  <Modal
    :model-value="open"
    :title="t('profile.advancedLogin.title')"
    size="lg"
    @update:model-value="onModelUpdate"
  >
    <div class="space-y-8">
      <!-- ── Passkeys ── -->
      <section class="space-y-4">
        <h3 class="text-base font-semibold text-white">
          {{ t('profile.advancedLogin.passkeys.title') }}
        </h3>

        <p v-if="!passkeySupported" class="text-sm text-white/50">
          {{ t('profile.advancedLogin.passkeys.unsupported') }}
        </p>

        <template v-else>
          <div class="flex items-end gap-2">
            <div class="flex-1">
              <Input
                v-model="passkeyName"
                :placeholder="t('profile.advancedLogin.passkeys.name')"
                @keydown.enter.prevent="onAddPasskey"
              />
            </div>
            <Button
              variant="primary"
              size="sm"
              :disabled="passkeyBusy || !passkeyName.trim()"
              @click="onAddPasskey"
            >
              {{ t('profile.advancedLogin.passkeys.add') }}
            </Button>
          </div>

          <div v-if="passkeysLoading && passkeys.length === 0" class="text-sm text-white/40">
            {{ t('common.loading') }}
          </div>
          <p v-else-if="passkeys.length === 0" class="text-sm text-white/50">
            {{ t('profile.advancedLogin.passkeys.empty') }}
          </p>
          <ul v-else class="space-y-2">
            <li
              v-for="pk in passkeys"
              :key="pk.id"
              class="flex items-start gap-3 rounded-lg bg-black/20 border border-white/5 p-3"
            >
              <div class="flex-1 min-w-0">
                <span class="block text-sm font-medium text-white truncate">{{ pk.name }}</span>
                <div class="text-xs text-white/50 mt-1">
                  {{ fmtDate(pk.created_at) }}
                  <template v-if="pk.last_used_at">
                    · {{ t('profile.advancedLogin.passkeys.lastUsed', { date: fmtDate(pk.last_used_at) }) }}
                  </template>
                </div>
              </div>
              <Button variant="secondary" size="sm" @click="onDeletePasskey(pk.id)">
                {{ t('profile.advancedLogin.passkeys.delete') }}
              </Button>
            </li>
          </ul>
        </template>
      </section>

      <!-- ── TLS certificate ── -->
      <section class="space-y-4">
        <h3 class="text-base font-semibold text-white">
          {{ t('profile.advancedLogin.cert.title') }}
        </h3>

        <details class="rounded-lg bg-black/20 border border-white/5 p-3">
          <summary class="text-sm text-white/70 cursor-pointer select-none">
            {{ t('profile.advancedLogin.cert.instructions') }}
          </summary>
          <div class="mt-3 space-y-2 text-xs text-white/60">
            <p>{{ t('profile.advancedLogin.cert.instructionsWindows') }}</p>
            <p>{{ t('profile.advancedLogin.cert.instructionsMacos') }}</p>
            <p>{{ t('profile.advancedLogin.cert.instructionsIos') }}</p>
            <p>{{ t('profile.advancedLogin.cert.instructionsAndroid') }}</p>
            <p>{{ t('profile.advancedLogin.cert.instructionsLinux') }}</p>
            <p>{{ t('profile.advancedLogin.cert.instructionsFirefox') }}</p>
            <div v-if="caInfo" class="mt-3 pt-3 border-t border-white/5 space-y-1.5">
              <p class="text-white/70">{{ t('profile.advancedLogin.cert.caFingerprintGuide') }}</p>
              <p>
                <span class="text-white/40">SHA-256:</span>
                <code class="font-mono text-cyan-300 break-all select-all">{{ fmtFingerprint(caInfo.fingerprint_sha256) }}</code>
              </p>
              <p>
                <span class="text-white/40">SHA-1:</span>
                <code class="font-mono text-cyan-300 break-all select-all">{{ fmtFingerprint(caInfo.fingerprint_sha1) }}</code>
              </p>
            </div>
          </div>
        </details>

        <div class="flex items-end gap-2">
          <div class="flex-1">
            <Input
              v-model="certName"
              :placeholder="t('profile.advancedLogin.cert.name')"
              @keydown.enter.prevent="onIssueCert"
            />
          </div>
          <Button
            variant="primary"
            size="sm"
            :disabled="certBusy || !certName.trim()"
            @click="onIssueCert"
          >
            {{ t('profile.advancedLogin.cert.issue') }}
          </Button>
        </div>

        <!-- One-time password (cleared on close) -->
        <div
          v-if="issuedPassword"
          class="rounded-lg bg-warning/10 border border-warning/30 p-3 space-y-2"
        >
          <p class="text-xs text-white/70">{{ t('profile.advancedLogin.cert.password') }}</p>
          <div class="flex items-center gap-2 rounded-md bg-black/30 p-2 font-mono text-sm text-white break-all">
            <span class="flex-1">{{ issuedPassword }}</span>
            <button
              type="button"
              class="flex-shrink-0 p-1.5 rounded hover:bg-white/10 text-white/60 hover:text-white transition-colors"
              :aria-label="t('profile.advancedLogin.cert.passwordCopy')"
              @click="copyPassword"
            >
              <Check v-if="passwordCopied" class="size-4 text-success" />
              <Copy v-else class="size-4" />
            </button>
          </div>
          <p class="text-xs text-warning">{{ t('profile.advancedLogin.cert.passwordWarn') }}</p>
        </div>

        <!-- Cert list -->
        <div v-if="certsLoading && certs.length === 0" class="text-sm text-white/40">
          {{ t('common.loading') }}
        </div>
        <p v-else-if="certs.length === 0" class="text-sm text-white/50">
          {{ t('profile.advancedLogin.cert.empty') }}
        </p>
        <ul v-else class="space-y-2">
          <li
            v-for="c in certs"
            :key="c.id"
            class="flex items-start gap-3 rounded-lg bg-black/20 border border-white/5 p-3"
          >
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-white truncate">{{ c.name }}</span>
                <span
                  v-if="c.revoked_at"
                  class="text-[10px] uppercase tracking-wide text-destructive bg-destructive/10 border border-destructive/30 rounded px-1.5 py-0.5"
                >
                  {{ t('profile.advancedLogin.cert.revoked') }}
                </span>
              </div>
              <div class="text-xs text-white/50 mt-1">
                {{ fmtDate(c.created_at) }} ·
                {{ t('profile.advancedLogin.cert.expires', { date: fmtDate(c.not_after) }) }}
                <template v-if="c.last_used_at">
                  · {{ t('profile.advancedLogin.cert.lastUsed', { date: fmtDate(c.last_used_at) }) }}
                </template>
              </div>
            </div>
            <Button
              v-if="!c.revoked_at"
              variant="secondary"
              size="sm"
              @click="onRevokeCert(c.id)"
            >
              {{ t('profile.advancedLogin.cert.revoke') }}
            </Button>
          </li>
        </ul>

        <!-- Auto-login toggle -->
        <div class="flex items-start justify-between gap-4 pt-2 border-t border-white/5">
          <div class="flex-1">
            <p class="text-sm text-white">{{ t('profile.advancedLogin.cert.autoLogin') }}</p>
            <p v-if="!hasActiveCert" class="text-xs text-white/40 mt-1">
              {{ t('profile.advancedLogin.cert.autoLoginHint') }}
            </p>
          </div>
          <Switch
            :model-value="certAutoLogin"
            :disabled="!hasActiveCert || autoLoginBusy"
            @update:model-value="setCertAutoLogin"
          />
        </div>
      </section>

      <p v-if="error" class="text-sm text-destructive">{{ error }}</p>
    </div>
  </Modal>
</template>
