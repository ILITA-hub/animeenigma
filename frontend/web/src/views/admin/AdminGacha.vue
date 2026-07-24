<template>
  <!-- /admin/gacha — Gacha manager with Cards / Groups / Banners tabs -->
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">{{ $t('gacha.admin.title') }}</h1>
          <p class="text-white/60 text-sm mt-1">{{ $t('gacha.admin.desc') }}</p>
        </div>
      </div>

      <!-- Error banner -->
      <Alert v-if="pageError" variant="destructive" class="mb-4" dismissible @dismiss="pageError = null">
        {{ pageError }}
      </Alert>

      <!-- Tabs: Cards / Groups / Banners -->
      <Tabs v-model="activeTab" :tabs="tabDefs" variant="underline">

        <!-- ─── CARDS TAB ─────────────────────────────────────────────── -->
        <template #cards>
          <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
            <!-- Filters -->
            <div class="flex flex-wrap gap-2">
              <Select
                v-model="cardFilterRarity"
                :options="rarityFilterOptions"
                :placeholder="$t('gacha.admin.filter_rarity')"
                size="sm"
                class="w-32"
              />
              <Select
                v-model="cardFilterGroup"
                :options="groupFilterOptions"
                :placeholder="$t('gacha.admin.filter_group')"
                size="sm"
                class="w-40"
              />
              <label class="flex items-center gap-1.5 text-sm text-white/70 cursor-pointer select-none">
                <Checkbox v-model="cardFilterEnabled" />
                {{ $t('gacha.admin.filter_enabled_only') }}
              </label>
            </div>
            <div class="flex gap-2">
              <Button size="sm" variant="outline" data-testid="bulk-upload-btn" @click="showBulkUpload = true">
                <Upload class="size-4 mr-1" />
                {{ $t('gacha.admin.bulk_upload_btn') }}
              </Button>
              <Button size="sm" @click="openCardCreate">
                + {{ $t('gacha.admin.card_create') }}
              </Button>
            </div>
          </div>

          <!-- Bulk actions bar (visible while a selection exists) -->
          <div
            v-if="selectedIds.size > 0"
            class="glass-card flex flex-wrap items-center gap-2 px-3 py-2 mb-3"
            data-testid="bulk-actions-bar"
          >
            <span class="text-sm text-white/80 font-medium">
              {{ $t('gacha.admin.bulk_selected', { n: selectedIds.size }) }}
            </span>
            <Select
              v-model="bulkRarity"
              :options="rarityOptions"
              :placeholder="$t('gacha.admin.bulk_set_rarity')"
              size="sm"
              class="w-28"
              data-testid="bulk-rarity-select"
            />
            <div class="flex items-center gap-1">
              <Input v-model="bulkName" size="sm" :placeholder="$t('gacha.admin.bulk_set_name_placeholder')" class="w-36" />
              <Button size="sm" variant="outline" :disabled="!bulkName.trim() || bulkBusy" data-testid="bulk-name-apply" @click="applyBulk({ name: bulkName.trim() })">
                {{ $t('gacha.admin.bulk_apply') }}
              </Button>
            </div>
            <div class="flex items-center gap-1">
              <Input v-model="bulkSource" size="sm" :placeholder="$t('gacha.admin.bulk_set_source_placeholder')" class="w-36" />
              <Button size="sm" variant="outline" :disabled="!bulkSource.trim() || bulkBusy" data-testid="bulk-source-apply" @click="applyBulk({ source_title: bulkSource.trim() })">
                {{ $t('gacha.admin.bulk_apply') }}
              </Button>
            </div>
            <Select
              v-model="bulkGroup"
              :options="bulkGroupOptions"
              :placeholder="$t('gacha.admin.bulk_add_to_group')"
              size="sm"
              class="w-36"
              data-testid="bulk-group-select"
            />
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-enable-btn" @click="applyBulk({ enabled: true })">
              {{ $t('gacha.admin.bulk_enable') }}
            </Button>
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-disable-btn" @click="applyBulk({ enabled: false })">
              {{ $t('gacha.admin.bulk_disable') }}
            </Button>
            <Button size="sm" variant="outline" :disabled="bulkBusy" data-testid="bulk-back-btn" @click="showBulkBack = true">
              {{ $t('gacha.admin.bulk_back_btn') }}
            </Button>
            <Button size="sm" variant="destructive" :disabled="bulkBusy" data-testid="bulk-delete-btn" @click="confirmBulkDelete">
              {{ $t('gacha.admin.bulk_delete') }}
            </Button>
          </div>

          <!-- Loading -->
          <div v-if="loadingCards" class="flex justify-center py-10">
            <Spinner />
          </div>

          <!-- Empty -->
          <div v-else-if="filteredCards.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            {{ $t('gacha.admin.cards_empty') }}
          </div>

          <!-- Cards table -->
          <div v-else class="glass-card overflow-x-auto" data-testid="cards-tab-table">
            <table class="w-full text-sm text-white">
              <thead class="bg-black/40 backdrop-blur text-white/70 text-xs uppercase">
                <tr>
                  <th class="px-3 py-2 w-8">
                    <Checkbox :model-value="allSelected" data-testid="select-all" @update:model-value="toggleSelectAll" />
                  </th>
                  <th class="px-3 py-2 text-left w-12">{{ $t('gacha.admin.card_image') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_name') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_source') }}</th>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.card_rarity') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.card_enabled') }}</th>
                  <th class="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="card in filteredCards"
                  :key="card.id"
                  class="border-t border-white/10 hover:bg-white/5"
                >
                  <td class="px-3 py-2">
                    <Checkbox :model-value="selectedIds.has(card.id)" :data-testid="`row-select-${card.id}`" @update:model-value="toggleSelect(card.id)" />
                  </td>
                  <td class="px-3 py-2">
                    <img
                      :src="cardImageUrl(card.image_path)"
                      :alt="card.name"
                      class="w-8 h-10 object-cover rounded"
                    />
                  </td>
                  <td class="px-3 py-2 font-medium cursor-pointer" @click="startInlineEdit(card, 'name')">
                    <Input
                      v-if="isEditing(card.id, 'name')"
                      v-model="inlineValue"
                      size="sm"
                      autofocus
                      @keyup.enter="commitInlineEdit"
                      @keyup.esc="cancelInlineEdit"
                      @blur="commitInlineEdit"
                      @click.stop
                    />
                    <template v-else>
                      {{ card.name }}
                      <span
                        v-if="!card.enabled"
                        class="ml-1.5 text-[10px] uppercase tracking-wide text-white/50 border border-white/20 rounded px-1 py-px align-middle"
                      >
                        {{ $t('gacha.admin.draft_badge') }}
                      </span>
                    </template>
                  </td>
                  <td class="px-3 py-2 text-white/60 text-xs cursor-pointer" @click="startInlineEdit(card, 'source_title')">
                    <Input
                      v-if="isEditing(card.id, 'source_title')"
                      v-model="inlineValue"
                      size="sm"
                      autofocus
                      @keyup.enter="commitInlineEdit"
                      @keyup.esc="cancelInlineEdit"
                      @blur="commitInlineEdit"
                      @click.stop
                    />
                    <template v-else>{{ card.source_title }}</template>
                  </td>
                  <td class="px-3 py-2">
                    <Select
                      :model-value="card.rarity"
                      :options="rarityOptions"
                      size="sm"
                      class="w-20"
                      @update:model-value="v => onInlineRarity(card, v as Rarity)"
                    />
                  </td>
                  <td class="px-3 py-2 text-center">
                    <Checkbox :model-value="card.enabled" @update:model-value="v => onInlineEnabled(card, v as boolean)" />
                  </td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <Button variant="ghost" size="sm" @click="openCardEdit(card)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button variant="ghost" size="sm" class="text-destructive" @click="confirmCardDelete(card)">
                      <Trash2 class="size-4" />
                    </Button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>

        <!-- ─── GROUPS TAB ───────────────────────────────────────────────── -->
        <template #groups>
          <div class="flex justify-end mb-4">
            <Button size="sm" @click="openGroupCreate">
              + {{ $t('gacha.admin.group_create') }}
            </Button>
          </div>

          <div v-if="loadingGroups" class="flex justify-center py-10">
            <Spinner />
          </div>
          <div v-else-if="groups.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            {{ $t('gacha.admin.groups_empty') }}
          </div>
          <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            <div
              v-for="group in groups"
              :key="group.id"
              class="glass-card p-4"
            >
              <div class="flex items-center justify-between mb-3">
                <span class="font-semibold text-white">{{ group.name }}</span>
                <div class="flex gap-1">
                  <Button variant="ghost" size="sm" @click="openGroupEdit(group)">
                    <Pencil class="size-4" />
                  </Button>
                  <Button variant="ghost" size="sm" class="text-destructive" @click="confirmGroupDelete(group)">
                    <Trash2 class="size-4" />
                  </Button>
                </div>
              </div>
              <p class="text-white/50 text-xs">{{ group.name }}</p>
            </div>
          </div>
        </template>

        <!-- ─── BANNERS TAB ──────────────────────────────────────────────── -->
        <template #banners>
          <div class="flex justify-end mb-4">
            <Button size="sm" @click="openBannerCreate">
              + {{ $t('gacha.admin.banner_create') }}
            </Button>
          </div>

          <div v-if="loadingBanners" class="flex justify-center py-10">
            <Spinner />
          </div>
          <div v-else-if="banners.length === 0" class="glass-card p-8 text-center text-muted-foreground">
            {{ $t('gacha.admin.banners_empty') }}
          </div>
          <div v-else class="glass-card overflow-x-auto">
            <table class="w-full text-sm text-white">
              <thead class="bg-black/40 backdrop-blur text-white/70 text-xs uppercase">
                <tr>
                  <th class="px-3 py-2 text-left">{{ $t('gacha.admin.banner_name') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.banner_standard') }}</th>
                  <th class="px-3 py-2 text-center">{{ $t('gacha.admin.banner_enabled') }}</th>
                  <th class="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="banner in banners"
                  :key="banner.id"
                  class="border-t border-white/10 hover:bg-white/5"
                >
                  <td class="px-3 py-2 font-medium">{{ banner.name }}</td>
                  <td class="px-3 py-2 text-center">
                    <span class="text-xs">{{ banner.is_standard ? '✓' : '–' }}</span>
                  </td>
                  <td class="px-3 py-2 text-center">
                    <span :class="banner.enabled ? 'text-teal-400 text-xs' : 'text-muted-foreground text-xs'">
                      {{ banner.enabled ? '✓' : '–' }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-right space-x-2">
                    <Button variant="ghost" size="sm" @click="openBannerEdit(banner)">
                      <Pencil class="size-4" />
                    </Button>
                    <Button variant="ghost" size="sm" class="text-destructive" @click="confirmBannerDelete(banner)">
                      <Trash2 class="size-4" />
                    </Button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </Tabs>
    </div>
  </div>

  <!-- ─── CARD CREATE/EDIT DIALOG ───────────────────────────────────────── -->
  <Modal
    v-model="showCardDialog"
    :title="editCard ? $t('gacha.admin.card_edit') : $t('gacha.admin.card_create')"
    closable
    size="lg"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_name') }}</label>
        <Input v-model="cardForm.name" placeholder="Character name" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_source') }}</label>
        <Input v-model="cardForm.source_title" placeholder="Source anime title" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_rarity') }}</label>
        <Select v-model="cardForm.rarity" :options="rarityOptions" />
      </div>
      <!-- Groups multiselect (M4) -->
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_groups') }}</label>
        <div class="flex flex-wrap gap-1.5">
          <label
            v-for="group in groups"
            :key="group.id"
            class="inline-flex items-center gap-1.5 cursor-pointer select-none text-sm text-white/70"
          >
            <Checkbox
              :model-value="cardForm.groupIds.includes(group.id)"
              @update:model-value="toggleCardGroup(group.id)"
            />
            {{ group.name }}
          </label>
        </div>
      </div>
      <div class="flex items-center gap-2">
        <Checkbox v-model="cardForm.enabled" />
        <label class="text-white/70 text-sm cursor-pointer">{{ $t('gacha.admin.card_enabled') }}</label>
      </div>

      <!-- Image upload: file OR URL -->
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_image') }}</label>
        <!-- Image preview -->
        <div v-if="imagePreview" class="mb-2">
          <img
            :src="imagePreview"
            alt="Preview"
            class="w-20 h-28 object-cover rounded border border-white/20"
          />
        </div>
        <div class="flex flex-col gap-2">
          <!-- File upload -->
          <div class="flex items-center gap-2">
            <label
              class="inline-flex items-center gap-1.5 cursor-pointer rounded-xl border border-white/20 bg-white/5 hover:bg-white/10 px-3 py-1.5 text-sm font-medium text-white transition"
            >
              <Upload class="size-4" aria-hidden="true" />
              {{ $t('gacha.admin.card_image_or') }}
              <input
                ref="fileInput"
                type="file"
                accept="image/*"
                class="sr-only"
                @change="onFileChange"
              />
            </label>
          </div>
          <!-- URL input -->
          <Input
            v-model="cardForm.imageUrl"
            :placeholder="$t('gacha.admin.card_image_url_placeholder')"
            @blur="onImageUrlBlur"
          />
        </div>
        <p v-if="uploadError" class="text-destructive text-xs mt-1">{{ $t('gacha.admin.upload_error') }}</p>
        <p v-if="uploading" class="text-muted-foreground text-xs mt-1">{{ $t('gacha.admin.upload_uploading') }}</p>
      </div>

      <!-- Card back upload: optional «Рубашка» (file OR URL) -->
      <div data-testid="card-back-slot">
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.card_back_image') }}</label>
        <p class="text-white/40 text-xs mb-2">{{ $t('gacha.admin.card_back_hint') }}</p>
        <!-- Back preview -->
        <div v-if="backPreview" class="mb-2">
          <img
            :src="backPreview"
            alt="Card back preview"
            class="w-20 h-28 object-cover rounded border border-white/20"
          />
        </div>
        <div class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <label
              class="inline-flex items-center gap-1.5 cursor-pointer rounded-xl border border-white/20 bg-white/5 hover:bg-white/10 px-3 py-1.5 text-sm font-medium text-white transition"
            >
              <Upload class="size-4" aria-hidden="true" />
              {{ $t('gacha.admin.card_image_or') }}
              <input
                type="file"
                accept="image/*"
                class="sr-only"
                @change="onBackFileChange"
              />
            </label>
          </div>
          <Input
            v-model="cardForm.backUrl"
            :placeholder="$t('gacha.admin.card_image_url_placeholder')"
            @blur="onBackUrlBlur"
          />
        </div>
        <p v-if="backError" class="text-destructive text-xs mt-1">{{ $t('gacha.admin.upload_error') }}</p>
        <p v-if="uploadingBack" class="text-muted-foreground text-xs mt-1">{{ $t('gacha.admin.upload_uploading') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showCardDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button :disabled="savingCard" @click="saveCard">
          {{ $t('gacha.admin.card_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── GROUP EDIT DIALOG ─────────────────────────────────────────────── -->
  <!-- Same two-view pattern as the banner dialog: form view ↔ card picker.
       groupPickerOpen switches which view is rendered inside the same Modal. -->
  <Modal
    v-model="showGroupDialog"
    :title="groupPickerOpen
      ? $t('gacha.admin.banner_picker_title')
      : (editGroup ? $t('gacha.admin.group_rename') : $t('gacha.admin.group_create'))"
    closable
    size="xl"
  >
    <!-- ── PICKER VIEW ──────────────────────────────────────────────────── -->
    <GachaCardPicker
      v-if="groupPickerOpen"
      ref="groupPickerRef"
      :exclude-ids="groupCurrentCardIds"
      :all-cards="cards"
      :groups="groups"
      :already-in-label="$t('gacha.admin.group_picker_already_in_group')"
      :search="groupPickerSearch"
      :selected="groupPickerSelected"
      data-testid="group-card-picker"
      @update:search="groupPickerSearch = $event"
      @update:selected="groupPickerSelected = $event"
      @confirm="onGroupPickerConfirm"
      @cancel="closeGroupPicker"
    />

    <!-- ── FORM VIEW ───────────────────────────────────────────────────── -->
    <div v-else class="space-y-4">
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.group_name') }}</label>
        <Input v-model="groupForm.name" placeholder="Group name" />
      </div>

      <!-- Group cards section (edit mode only) -->
      <div v-if="editGroup">
        <p class="text-white/70 text-xs mb-2">{{ $t('gacha.admin.group_cards_section') }}</p>

        <!-- Current pool loading -->
        <div v-if="loadingGroupPool" class="text-muted-foreground text-xs mb-2">
          {{ $t('gacha.admin.upload_uploading') }}
        </div>

        <!-- Current member cards grid (thumbnail + name + rarity + remove) -->
        <div v-else-if="groupCurrentCardIds.length > 0" class="mb-3">
          <div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-2 max-h-52 overflow-y-auto pr-1">
            <div
              v-for="cid in groupCurrentCardIds"
              :key="cid"
              class="relative flex flex-col items-center gap-1 rounded-xl border border-white/20 bg-white/5"
            >
              <img
                :src="cardImageUrl(cardById(cid)?.image_path ?? '')"
                :alt="cardNameById(cid)"
                class="w-full aspect-[3/4] object-cover rounded-t-xl"
              />
              <div class="w-full px-1.5 pb-1">
                <p class="text-white text-xs font-medium truncate leading-tight">{{ cardNameById(cid) || cid }}</p>
                <span
                  v-if="cardById(cid)"
                  :class="['text-xs font-semibold px-1 py-0.5 rounded', rarityBadgeClass(cardById(cid)!.rarity)]"
                >
                  {{ cardById(cid)!.rarity }}
                </span>
              </div>
              <!-- Remove button — bespoke-keep: 20px circular black→red image-overlay affordance; Button (32px, rounded-xl, solid-bg variants) can't model it -->
              <button
                type="button"
                class="absolute top-1 right-1 size-5 rounded-full bg-black/60 hover:bg-destructive/80 flex items-center justify-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
                :aria-label="`Remove ${cardNameById(cid)}`"
                @click="removeGroupCard(cid)"
              >
                <X class="size-3 text-white" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
        <p v-else class="text-muted-foreground text-xs mb-3">{{ $t('gacha.admin.group_pool_empty') }}</p>

        <!-- Add cards button -->
        <Button
          variant="outline"
          size="sm"
          data-testid="open-group-card-picker-btn"
          @click="openGroupPicker"
        >
          + {{ $t('gacha.admin.group_add_cards') }}
        </Button>
      </div>

      <!-- New group hint: cards disabled until group saved -->
      <div v-else class="flex items-center gap-2 text-white/40 text-xs">
        <Info class="size-4 shrink-0" aria-hidden="true" />
        {{ $t('gacha.admin.group_new_hint') }}
      </div>
    </div>

    <template #footer>
      <!-- PICKER FOOTER -->
      <div v-if="groupPickerOpen" class="flex items-center justify-between w-full gap-3">
        <div class="flex items-center gap-3">
          <Button variant="ghost" size="sm" @click="closeGroupPicker">
            ← {{ $t('gacha.admin.banner_picker_back') }}
          </Button>
          <button
            type="button"
            class="text-xs text-white/60 hover:text-white transition-colors"
            @click="selectAllGroupPickerVisible"
          >
            {{ $t('gacha.admin.banner_picker_select_all') }}
          </button>
          <span v-if="groupPickerSelected.size > 0" class="text-xs text-white/60">
            {{ $t('gacha.admin.banner_picker_selected', { n: groupPickerSelected.size }) }}
          </span>
        </div>
        <Button
          :disabled="groupPickerSelected.size === 0 || addingGroupCards"
          data-testid="group-picker-add-btn"
          @click="confirmGroupPickerAdd"
        >
          {{ $t('gacha.admin.banner_picker_add_btn', { n: groupPickerSelected.size }) }}
        </Button>
      </div>

      <!-- FORM FOOTER -->
      <div v-else class="flex justify-end gap-2">
        <Button variant="outline" @click="showGroupDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button :disabled="savingGroup" @click="saveGroup">
          {{ $t('gacha.admin.card_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── BANNER CREATE/EDIT DIALOG ────────────────────────────────────── -->
  <!-- Single Modal with two views: form view ↔ card picker view.
       This avoids stacked DialogPortal issues (Reka's DismissableLayer leaks
       pointer-events when two modals stack and the inner one closes first).
       bannerPickerOpen switches which view is rendered inside the same Modal. -->
  <Modal
    v-model="showBannerDialog"
    :title="bannerPickerOpen ? $t('gacha.admin.banner_picker_title') : (editBanner ? $t('gacha.admin.banner_edit') : $t('gacha.admin.banner_create'))"
    closable
    size="xl"
  >
    <!-- ── PICKER VIEW ──────────────────────────────────────────────────── -->
    <GachaCardPicker
      v-if="bannerPickerOpen"
      ref="bannerPickerRef"
      :exclude-ids="bannerCurrentCardIds"
      :all-cards="cards"
      :groups="groups"
      :already-in-label="$t('gacha.admin.banner_picker_already_in_banner')"
      :search="pickerSearch"
      :selected="pickerSelected"
      data-testid="banner-card-picker"
      @update:search="pickerSearch = $event"
      @update:selected="pickerSelected = $event"
      @confirm="onBannerPickerConfirm"
      @cancel="closeCardPicker"
    />

    <!-- ── FORM VIEW ───────────────────────────────────────────────────── -->
    <div v-else class="space-y-4">
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_name') }}</label>
        <Input v-model="bannerForm.name" placeholder="Banner name" />
      </div>
      <div>
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_description') }}</label>
        <Input v-model="bannerForm.description" placeholder="Short description" />
      </div>
      <div class="flex flex-wrap gap-4">
        <label class="flex items-center gap-2">
          <Checkbox v-model="bannerForm.is_standard" />
          <span class="text-white/70 text-sm">{{ $t('gacha.admin.banner_standard') }}</span>
        </label>
        <label class="flex items-center gap-2">
          <Checkbox v-model="bannerForm.enabled" />
          <span class="text-white/70 text-sm">{{ $t('gacha.admin.banner_enabled') }}</span>
        </label>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_active_from') }}</label>
          <input
            v-model="bannerForm.active_from"
            type="datetime-local"
            class="w-full bg-white/5 border border-white/10 text-white placeholder-white/30 transition-all duration-200 focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 px-4 py-3 text-base rounded-xl"
          />
        </div>
        <div>
          <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_active_to') }}</label>
          <input
            v-model="bannerForm.active_to"
            type="datetime-local"
            class="w-full bg-white/5 border border-white/10 text-white placeholder-white/30 transition-all duration-200 focus:outline-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 px-4 py-3 text-base rounded-xl"
          />
        </div>
      </div>

      <!-- Backdrop «Задник» — single banner image slot -->
      <div data-testid="banner-backdrop-slot">
        <label class="block text-white/70 text-xs mb-1">{{ $t('gacha.admin.banner_backdrop') }}</label>
        <div v-if="bannerForm.backdrop_path" class="mb-2">
          <img
            :src="cardImageUrl(bannerForm.backdrop_path)"
            alt="Banner backdrop preview"
            class="w-full h-24 object-cover rounded border border-white/20"
          />
        </div>
        <div class="flex flex-col gap-2">
          <label
            class="inline-flex items-center gap-1.5 cursor-pointer rounded-xl border border-white/20 bg-white/5 hover:bg-white/10 px-3 py-1.5 text-sm font-medium text-white transition"
          >
            <Upload class="size-4" aria-hidden="true" />
            {{ $t('gacha.admin.card_image_or') }}
            <input type="file" accept="image/*" class="sr-only" @change="onBannerBackdropFile" />
          </label>
          <Input
            v-model="bannerBackdropUrl"
            :placeholder="$t('gacha.admin.card_image_url_placeholder')"
            @blur="onBannerBackdropUrlBlur"
          />
        </div>
        <p v-if="bannerBackdropError" class="text-destructive text-xs mt-1">{{ $t('gacha.admin.upload_error') }}</p>
        <p v-if="uploadingBannerBackdrop" class="text-muted-foreground text-xs mt-1">{{ $t('gacha.admin.upload_uploading') }}</p>
      </div>

      <!-- Banner cards section -->
      <div v-if="editBanner">
        <p class="text-white/70 text-xs mb-2">{{ $t('gacha.admin.banner_cards_section') }}</p>

        <!-- Current pool loading -->
        <div v-if="loadingBannerPool" class="text-muted-foreground text-xs mb-2">
          {{ $t('gacha.admin.upload_uploading') }}
        </div>

        <!-- Current pool grid (thumbnail + name + rarity + remove) -->
        <div v-else-if="bannerCurrentCardIds.length > 0" class="mb-3">
          <div class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-2 max-h-52 overflow-y-auto pr-1">
            <div
              v-for="cid in bannerCurrentCardIds"
              :key="cid"
              class="relative flex flex-col items-center gap-1 rounded-xl border border-white/20 bg-white/5"
            >
              <img
                :src="cardImageUrl(cardById(cid)?.image_path ?? '')"
                :alt="cardNameById(cid)"
                class="w-full aspect-[3/4] object-cover rounded-t-xl"
              />
              <div class="w-full px-1.5 pb-1">
                <p class="text-white text-xs font-medium truncate leading-tight">{{ cardNameById(cid) || cid }}</p>
                <span
                  v-if="cardById(cid)"
                  :class="['text-xs font-semibold px-1 py-0.5 rounded', rarityBadgeClass(cardById(cid)!.rarity)]"
                >
                  {{ cardById(cid)!.rarity }}
                </span>
              </div>
              <!-- Remove button — bespoke-keep: 20px circular black→red image-overlay affordance; Button (32px, rounded-xl, solid-bg variants) can't model it -->
              <button
                type="button"
                class="absolute top-1 right-1 size-5 rounded-full bg-black/60 hover:bg-destructive/80 flex items-center justify-center transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
                :aria-label="`Remove ${cardNameById(cid)}`"
                @click="removeBannerCard(cid)"
              >
                <X class="size-3 text-white" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
        <p v-else class="text-muted-foreground text-xs mb-3">{{ $t('gacha.admin.banner_pool_empty') }}</p>

        <!-- Add cards button -->
        <Button variant="outline" size="sm" @click="openCardPicker" data-testid="open-card-picker-btn">
          + {{ $t('gacha.admin.banner_add_cards_btn') }}
        </Button>
      </div>

      <!-- New banner hint: cards disabled until banner saved -->
      <div v-else class="flex items-center gap-2 text-white/40 text-xs">
        <Info class="size-4 shrink-0" aria-hidden="true" />
        {{ $t('gacha.admin.banner_new_hint') }}
      </div>
    </div>

    <template #footer>
      <!-- PICKER FOOTER -->
      <div v-if="bannerPickerOpen" class="flex items-center justify-between w-full gap-3">
        <div class="flex items-center gap-3">
          <Button variant="ghost" size="sm" @click="closeCardPicker">
            ← {{ $t('gacha.admin.banner_picker_back') }}
          </Button>
          <button
            type="button"
            class="text-xs text-white/60 hover:text-white transition-colors"
            @click="selectAllPickerVisible"
          >
            {{ $t('gacha.admin.banner_picker_select_all') }}
          </button>
          <span v-if="pickerSelected.size > 0" class="text-xs text-white/60">
            {{ $t('gacha.admin.banner_picker_selected', { n: pickerSelected.size }) }}
          </span>
        </div>
        <Button
          :disabled="pickerSelected.size === 0 || addingPickerCards"
          data-testid="picker-add-btn"
          @click="confirmPickerAdd"
        >
          {{ $t('gacha.admin.banner_picker_add_btn', { n: pickerSelected.size }) }}
        </Button>
      </div>

      <!-- FORM FOOTER -->
      <div v-else class="flex justify-end gap-2">
        <Button variant="outline" @click="showBannerDialog = false">
          {{ $t('gacha.admin.banner_cancel') }}
        </Button>
        <Button :disabled="savingBanner" @click="saveBanner">
          {{ $t('gacha.admin.banner_save') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── DELETE CONFIRM DIALOG ─────────────────────────────────────────── -->
  <Modal v-model="showDeleteDialog" :title="deleteTarget?.label ?? ''" closable>
    <p class="text-white/80">{{ deleteTarget?.confirmMsg }}</p>
    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="showDeleteDialog = false">
          {{ $t('gacha.admin.card_cancel') }}
        </Button>
        <Button variant="destructive" :disabled="deleting" @click="runDelete">
          {{ $t('gacha.admin.card_delete') }}
        </Button>
      </div>
    </template>
  </Modal>

  <!-- ─── BULK UPLOAD DIALOG ────────────────────────────────────────────── -->
  <GachaBulkUpload v-model="showBulkUpload" />

  <!-- ─── BULK CARD-BACK DIALOG ─────────────────────────────────────────── -->
  <Modal v-model="showBulkBack" :title="$t('gacha.admin.bulk_back_title')" closable>
    <p class="text-white/70 text-sm mb-3">{{ $t('gacha.admin.bulk_back_hint') }}</p>
    <div class="flex items-start gap-3">
      <img v-if="bulkBackPreview" :src="bulkBackPreview" alt="" class="w-16 h-20 object-cover rounded border border-white/20" />
      <div class="flex-1 space-y-2">
        <input ref="bulkBackFileInput" type="file" accept="image/*" class="hidden" @change="onBulkBackFile" />
        <Button size="sm" variant="outline" :disabled="bulkBackUploading" @click="bulkBackFileInput?.click()">
          <Upload class="size-4 mr-1" /> {{ $t('gacha.admin.card_image_or') }}
        </Button>
        <Input v-model="bulkBackUrl" :placeholder="$t('gacha.admin.card_image_url_placeholder')" @blur="onBulkBackUrl" />
        <Alert v-if="bulkBackError" variant="destructive">{{ $t('gacha.admin.upload_error') }}</Alert>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <Button variant="outline" :disabled="bulkBusy" data-testid="bulk-back-reset" @click="applyBulkBack('')">
          {{ $t('gacha.admin.bulk_back_reset') }}
        </Button>
        <Button :disabled="!bulkBackPath || bulkBackUploading || bulkBusy" data-testid="bulk-back-apply" @click="applyBulkBack(bulkBackPath)">
          {{ $t('gacha.admin.bulk_back_apply', { n: selectedIds.size }) }}
        </Button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pencil, Trash2, Upload, X, Info } from 'lucide-vue-next'
import {
  gachaAdminApi,
  cardImageUrl,
  type GachaCard,
  type GachaGroup,
  type GachaBanner,
  type Rarity,
  type BulkCardSet,
} from '@/api/gacha'
import Tabs from '@/components/ui/Tabs.vue'
import Modal from '@/components/ui/Modal.vue'
import Input from '@/components/ui/Input.vue'
import Button from '@/components/ui/Button.vue'
import Select from '@/components/ui/Select.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Spinner from '@/components/ui/Spinner.vue'
import Alert from '@/components/ui/Alert.vue'
import GachaCardPicker from '@/components/admin/GachaCardPicker.vue'
import GachaBulkUpload from '@/components/admin/GachaBulkUpload.vue'

const { t } = useI18n()

// ── Tabs ──────────────────────────────────────────────────────────────────────
const activeTab = ref<'cards' | 'groups' | 'banners'>('cards')
const tabDefs = [
  { value: 'cards',   label: t('gacha.admin.tab_cards') },
  { value: 'groups',  label: t('gacha.admin.tab_groups') },
  { value: 'banners', label: t('gacha.admin.tab_banners') },
]

// ── Shared state ──────────────────────────────────────────────────────────────
const pageError = ref<string | null>(null)

// ── Cards state ───────────────────────────────────────────────────────────────
const cards = ref<GachaCard[]>([])
const loadingCards = ref(false)
const cardFilterRarity = ref<string>('all')
const cardFilterGroup = ref<string>('all')
const cardFilterEnabled = ref(false)

const rarityFilterOptions = [
  { value: 'all', label: t('gacha.admin.filter_all') },
  { value: 'N', label: 'N' },
  { value: 'R', label: 'R' },
  { value: 'SR', label: 'SR' },
  { value: 'SSR', label: 'SSR' },
]

const filteredCards = computed(() => {
  return cards.value.filter(c => {
    if (cardFilterRarity.value !== 'all' && c.rarity !== cardFilterRarity.value) return false
    if (cardFilterEnabled.value && !c.enabled) return false
    // M6: group filter is applied server-side; no client-side group filter
    return true
  })
})

// ── Bulk upload dialog ────────────────────────────────────────────────────────
const showBulkUpload = ref(false)
// Refetch on close — the dialog created draft cards behind the table's back.
watch(showBulkUpload, open => {
  if (!open) void loadCards()
})

// ── Bulk selection + actions ──────────────────────────────────────────────────
const selectedIds = ref<Set<string>>(new Set())
const bulkBusy = ref(false)
const bulkName = ref('')
const bulkSource = ref('')
const bulkRarity = ref('')
const bulkGroup = ref('')

const allSelected = computed(() =>
  filteredCards.value.length > 0 && filteredCards.value.every(c => selectedIds.value.has(c.id)))

function toggleSelect(id: string) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

function toggleSelectAll() {
  selectedIds.value = allSelected.value ? new Set() : new Set(filteredCards.value.map(c => c.id))
}

// Selection references filtered rows; changing filters invalidates it.
watch([cardFilterRarity, cardFilterGroup, cardFilterEnabled], () => {
  selectedIds.value = new Set()
})

async function applyBulk(set: BulkCardSet) {
  if (selectedIds.value.size === 0) return
  bulkBusy.value = true
  pageError.value = null
  try {
    await gachaAdminApi.bulkUpdateCards(Array.from(selectedIds.value), set)
    bulkName.value = ''
    bulkSource.value = ''
    await loadCards()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    bulkBusy.value = false
  }
}

// Selects apply immediately on pick, then reset to '' so the placeholder returns.
watch(bulkRarity, v => {
  if (!v) return
  void applyBulk({ rarity: v as Rarity }).finally(() => { bulkRarity.value = '' })
})

watch(bulkGroup, v => {
  if (!v || selectedIds.value.size === 0) {
    if (v) bulkGroup.value = ''
    return
  }
  bulkBusy.value = true
  gachaAdminApi.addCardsToGroup(v, Array.from(selectedIds.value))
    .catch((err: unknown) => { pageError.value = extractMessage(err) })
    .finally(() => { bulkBusy.value = false; bulkGroup.value = '' })
})

const bulkGroupOptions = computed(() => groups.value.map(g => ({ value: g.id, label: g.name })))

function confirmBulkDelete() {
  const ids = Array.from(selectedIds.value)
  if (ids.length === 0) return
  deleteTarget.value = {
    label: t('gacha.admin.bulk_delete_label'),
    confirmMsg: t('gacha.admin.bulk_delete_confirm', { n: ids.length }),
    action: async () => {
      await gachaAdminApi.bulkDeleteCards(ids)
      selectedIds.value = new Set()
      await loadCards()
    },
  }
  showDeleteDialog.value = true
}

// ── Bulk card-back dialog ──────────────────────────────────────────────────────
const showBulkBack = ref(false)
const bulkBackPath = ref('')
const bulkBackUrl = ref('')
const bulkBackPreview = ref<string | null>(null)
const bulkBackUploading = ref(false)
const bulkBackError = ref(false)
const bulkBackFileInput = ref<HTMLInputElement | null>(null)

watch(showBulkBack, open => {
  if (open) {
    bulkBackPath.value = ''
    bulkBackUrl.value = ''
    bulkBackPreview.value = null
    bulkBackError.value = false
  }
})

async function onBulkBackFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  bulkBackUploading.value = true
  bulkBackError.value = false
  try {
    const res = await gachaAdminApi.uploadFile(file, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    bulkBackPath.value = path
    bulkBackPreview.value = path ? cardImageUrl(path) : null
  } catch {
    bulkBackError.value = true
  } finally {
    bulkBackUploading.value = false
  }
}

async function onBulkBackUrl() {
  const url = bulkBackUrl.value.trim()
  if (!url) return
  bulkBackUploading.value = true
  bulkBackError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl(url, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    bulkBackPath.value = path
    bulkBackPreview.value = path ? cardImageUrl(path) : url
  } catch {
    bulkBackError.value = true
  } finally {
    bulkBackUploading.value = false
  }
}

async function applyBulkBack(path: string) {
  await applyBulk({ back_path: path })
  showBulkBack.value = false
}

// ── Inline cell editing ───────────────────────────────────────────────────────
// Single-cell edits go through the bulk endpoint with one id: partial
// semantics — unlike updateCard (full replace), nothing else can be clobbered.
const inlineEdit = ref<{ id: string; field: 'name' | 'source_title' } | null>(null)
const inlineValue = ref('')

function isEditing(id: string, field: 'name' | 'source_title') {
  return inlineEdit.value?.id === id && inlineEdit.value?.field === field
}

function startInlineEdit(card: GachaCard, field: 'name' | 'source_title') {
  inlineEdit.value = { id: card.id, field }
  inlineValue.value = field === 'name' ? card.name : card.source_title
}

function cancelInlineEdit() {
  inlineEdit.value = null
}

async function commitInlineEdit() {
  const edit = inlineEdit.value
  if (!edit) return
  inlineEdit.value = null
  const card = cards.value.find(c => c.id === edit.id)
  if (!card) return
  const value = inlineValue.value.trim()
  if (edit.field === 'name' && !value) return // backend rejects empty names
  if (value === (edit.field === 'name' ? card.name : card.source_title)) return
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { [edit.field]: value })
    if (edit.field === 'name') card.name = value
    else card.source_title = value
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

async function onInlineRarity(card: GachaCard, rarity: Rarity) {
  if (rarity === card.rarity) return
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { rarity })
    card.rarity = rarity
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

async function onInlineEnabled(card: GachaCard, enabled: boolean) {
  try {
    await gachaAdminApi.bulkUpdateCards([card.id], { enabled })
    card.enabled = enabled
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

// ── Groups state ──────────────────────────────────────────────────────────────
const groups = ref<GachaGroup[]>([])
const loadingGroups = ref(false)

const groupFilterOptions = computed(() => [
  { value: 'all', label: t('gacha.admin.filter_all') },
  ...groups.value.map(g => ({ value: g.id, label: g.name })),
])

// ── Banners state ─────────────────────────────────────────────────────────────
const banners = ref<GachaBanner[]>([])
const loadingBanners = ref(false)

// ── Card dialog ───────────────────────────────────────────────────────────────
const showCardDialog = ref(false)
const editCard = ref<GachaCard | null>(null)
const savingCard = ref(false)
const uploading = ref(false)
const uploadError = ref(false)
const imagePreview = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const cardForm = ref({
  name: '',
  source_title: '',
  rarity: 'N' as Rarity,
  enabled: true,
  imagePath: '',   // final stored path (returned from upload or existing)
  imageUrl: '',    // user-typed URL
  backPath: '',    // optional card-back image key (new slot)
  backUrl: '',     // user-typed URL for the card back
  groupIds: [] as string[],  // selected group membership (M4)
})

// Card-back upload state (optional «Рубашка» slot, file-or-URL flow).
const uploadingBack = ref(false)
const backError = ref(false)
const backPreview = ref<string | null>(null)

function resetCardForm() {
  cardForm.value = { name: '', source_title: '', rarity: 'N', enabled: true, imagePath: '', imageUrl: '', backPath: '', backUrl: '', groupIds: [] }
  imagePreview.value = null
  uploadError.value = false
  uploading.value = false
  backPreview.value = null
  backError.value = false
  uploadingBack.value = false
}

function openCardCreate() {
  editCard.value = null
  resetCardForm()
  showCardDialog.value = true
}

function openCardEdit(card: GachaCard) {
  editCard.value = card
  cardForm.value = {
    name: card.name,
    source_title: card.source_title,
    rarity: card.rarity,
    enabled: card.enabled,
    imagePath: card.image_path,
    imageUrl: '',
    backPath: card.back_path ?? '',
    backUrl: '',
    groupIds: [],   // We don't have the current membership here; user selects from scratch
  }
  imagePreview.value = card.image_path ? cardImageUrl(card.image_path) : null
  backPreview.value = card.back_path ? cardImageUrl(card.back_path) : null
  uploadError.value = false
  uploading.value = false
  backError.value = false
  uploadingBack.value = false
  showCardDialog.value = true
}

async function onBackFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploadingBack.value = true
  backError.value = false
  try {
    const res = await gachaAdminApi.uploadFile(file, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.backPath = path
    backPreview.value = path ? cardImageUrl(path) : null
  } catch {
    backError.value = true
  } finally {
    uploadingBack.value = false
  }
}

async function onBackUrlBlur() {
  const url = cardForm.value.backUrl.trim()
  if (!url) return
  uploadingBack.value = true
  backError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl(url, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.backPath = path
    backPreview.value = path ? cardImageUrl(path) : url
  } catch {
    backError.value = true
    backPreview.value = url
  } finally {
    uploadingBack.value = false
  }
}

function toggleCardGroup(groupId: string) {
  const idx = cardForm.value.groupIds.indexOf(groupId)
  if (idx === -1) {
    cardForm.value.groupIds.push(groupId)
  } else {
    cardForm.value.groupIds.splice(idx, 1)
  }
}

async function onFileChange(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploading.value = true
  uploadError.value = false
  try {
    const res = await gachaAdminApi.uploadFile(file, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.imagePath = path
    imagePreview.value = path ? cardImageUrl(path) : null
  } catch {
    uploadError.value = true
  } finally {
    uploading.value = false
  }
}

async function onImageUrlBlur() {
  const url = cardForm.value.imageUrl.trim()
  if (!url) return
  uploading.value = true
  uploadError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl(url, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const path = data?.data?.image_path ?? ''
    cardForm.value.imagePath = path
    imagePreview.value = path ? cardImageUrl(path) : url
  } catch {
    uploadError.value = true
    imagePreview.value = url
  } finally {
    uploading.value = false
  }
}

async function saveCard() {
  savingCard.value = true
  pageError.value = null
  try {
    const payload = {
      name: cardForm.value.name,
      source_title: cardForm.value.source_title,
      rarity: cardForm.value.rarity,
      enabled: cardForm.value.enabled,
      image_path: cardForm.value.imagePath,
      back_path: cardForm.value.backPath,
      group_ids: cardForm.value.groupIds,
    }
    if (editCard.value) {
      // Update card fields + apply group membership diffs (M4)
      await gachaAdminApi.updateCard(editCard.value.id, payload)
      // Apply group membership changes: add to selected groups, the API
      // handles deduplication; remove path deferred (no full prior membership).
      if (cardForm.value.groupIds.length > 0) {
        for (const gid of cardForm.value.groupIds) {
          await gachaAdminApi.addCardsToGroup(gid, [editCard.value.id])
        }
      }
    } else {
      await gachaAdminApi.createCard(payload)
    }
    showCardDialog.value = false
    await loadCards()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingCard.value = false
  }
}

// ── Group dialog ──────────────────────────────────────────────────────────────
const showGroupDialog = ref(false)
const editGroup = ref<GachaGroup | null>(null)
const savingGroup = ref(false)
const groupForm = ref({ name: '' })

// Group card picker state (two-view, mirrors the banner picker pattern)
const groupPickerOpen = ref(false)
const groupCurrentCardIds = ref<string[]>([])
const loadingGroupPool = ref(false)
const addingGroupCards = ref(false)
const groupPickerSearch = ref('')
const groupPickerSelected = ref<Set<string>>(new Set())
const groupPickerRef = ref<InstanceType<typeof GachaCardPicker> | null>(null)

function openGroupCreate() {
  editGroup.value = null
  groupForm.value = { name: '' }
  groupCurrentCardIds.value = []
  groupPickerOpen.value = false
  showGroupDialog.value = true
}

/** Open group dialog for rename only (no card pool fetch).
 *  Also used as setup helper by openGroupEdit. */
function openGroupRename(group: GachaGroup) {
  editGroup.value = group
  groupForm.value = { name: group.name }
  groupCurrentCardIds.value = []
  groupPickerOpen.value = false
  showGroupDialog.value = true
}

/** Open group dialog in edit mode (rename + card management).
 *  Called from the pencil button in the groups grid. */
async function openGroupEdit(group: GachaGroup) {
  openGroupRename(group)
  // Fetch current member cards via listCards({group_id})
  loadingGroupPool.value = true
  try {
    const res = await gachaAdminApi.listCards({ group_id: group.id })
    const members = ((res as { data?: { data?: GachaCard[] } }).data?.data ?? [])
    groupCurrentCardIds.value = members.map(c => c.id)
  } catch {
    // non-fatal: pool stays empty
  } finally {
    loadingGroupPool.value = false
  }
}

function openGroupPicker() {
  groupPickerSearch.value = ''
  groupPickerSelected.value = new Set()
  groupPickerOpen.value = true
}

function closeGroupPicker() {
  groupPickerOpen.value = false
}

function selectAllGroupPickerVisible() {
  groupPickerRef.value?.selectAllVisible()
}

async function confirmGroupPickerAdd() {
  const ids = Array.from(groupPickerSelected.value)
  await onGroupPickerConfirm(ids)
}

async function onGroupPickerConfirm(ids: string[]) {
  if (!editGroup.value || ids.length === 0) return
  addingGroupCards.value = true
  pageError.value = null
  try {
    await gachaAdminApi.addCardsToGroup(editGroup.value.id, ids)
    // Merge into local pool
    for (const id of ids) {
      if (!groupCurrentCardIds.value.includes(id)) {
        groupCurrentCardIds.value.push(id)
      }
    }
    groupPickerSelected.value = new Set()
    groupPickerOpen.value = false
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    addingGroupCards.value = false
  }
}

async function removeGroupCard(cardId: string) {
  if (!editGroup.value) return
  try {
    await gachaAdminApi.removeCardFromGroup(editGroup.value.id, cardId)
    groupCurrentCardIds.value = groupCurrentCardIds.value.filter(id => id !== cardId)
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

// Reset group picker state when dialog closes
watch(showGroupDialog, (open) => {
  if (!open) {
    groupPickerOpen.value = false
    groupPickerSearch.value = ''
    groupPickerSelected.value = new Set()
  }
})

async function saveGroup() {
  savingGroup.value = true
  pageError.value = null
  try {
    if (editGroup.value) {
      await gachaAdminApi.renameGroup(editGroup.value.id, groupForm.value.name)
    } else {
      await gachaAdminApi.createGroup(groupForm.value.name)
    }
    showGroupDialog.value = false
    await loadGroups()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingGroup.value = false
  }
}

// ── Banner dialog ─────────────────────────────────────────────────────────────
const showBannerDialog = ref(false)
const editBanner = ref<GachaBanner | null>(null)
const savingBanner = ref(false)

// Backdrop upload state (file-or-URL, same flow as card image).
const bannerBackdropUrl = ref('')   // user-typed URL for the backdrop slot
const uploadingBannerBackdrop = ref(false)
const bannerBackdropError = ref(false)

async function onBannerBackdropFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploadingBannerBackdrop.value = true
  bannerBackdropError.value = false
  try {
    const res = await gachaAdminApi.uploadFile(file, 'banners')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    bannerForm.value.backdrop_path = data?.data?.image_path ?? ''
  } catch {
    bannerBackdropError.value = true
  } finally {
    uploadingBannerBackdrop.value = false
  }
}

async function onBannerBackdropUrlBlur() {
  const url = bannerBackdropUrl.value.trim()
  if (!url) return
  uploadingBannerBackdrop.value = true
  bannerBackdropError.value = false
  try {
    const res = await gachaAdminApi.uploadUrl(url, 'banners')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    bannerForm.value.backdrop_path = data?.data?.image_path ?? ''
  } catch {
    bannerBackdropError.value = true
  } finally {
    uploadingBannerBackdrop.value = false
  }
}
// M5: current pool for the banner being edited
const bannerCurrentCardIds = ref<string[]>([])
const loadingBannerPool = ref(false)

// ── Banner card picker state ──────────────────────────────────────────────────
// bannerPickerOpen switches the banner modal between form view and picker view.
// pickerSearch and pickerSelected are controlled by the parent (passed as props
// to GachaCardPicker) so that existing spec assertions on vm.pickerSearch and
// vm.pickerSelected continue to work without modification.
const bannerPickerOpen = ref(false)
const pickerSearch = ref('')
const pickerSelected = ref<Set<string>>(new Set())
const addingPickerCards = ref(false)
const bannerPickerRef = ref<InstanceType<typeof GachaCardPicker> | null>(null)

// pickerFilteredCards — computed here in the parent using the same logic as the
// picker component, so existing spec assertions on vm.pickerFilteredCards work.
const pickerRarity = ref('all')
const pickerFilteredCards = computed(() => {
  const q = pickerSearch.value.toLowerCase().trim()
  return cards.value.filter(c => {
    if (pickerRarity.value !== 'all' && c.rarity !== pickerRarity.value) return false
    if (q && !c.name.toLowerCase().includes(q) && !c.source_title.toLowerCase().includes(q)) return false
    return true
  })
})

function openCardPicker() {
  pickerSearch.value = ''
  pickerRarity.value = 'all'
  pickerSelected.value = new Set()
  bannerPickerOpen.value = true
}

function closeCardPicker() {
  bannerPickerOpen.value = false
}

function selectAllPickerVisible() {
  groupPickerRef.value?.selectAllVisible()
  // For banner picker: select all from pickerFilteredCards not already in banner
  const next = new Set(pickerSelected.value)
  for (const card of pickerFilteredCards.value) {
    if (!bannerCurrentCardIds.value.includes(card.id)) {
      next.add(card.id)
    }
  }
  pickerSelected.value = next
}

async function onBannerPickerConfirm(ids: string[]) {
  if (!editBanner.value || ids.length === 0) return
  addingPickerCards.value = true
  pageError.value = null
  try {
    await gachaAdminApi.addBannerCards(editBanner.value.id, ids)
    // Merge into local pool
    for (const id of ids) {
      if (!bannerCurrentCardIds.value.includes(id)) {
        bannerCurrentCardIds.value.push(id)
      }
    }
    pickerSelected.value = new Set()
    bannerPickerOpen.value = false
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    addingPickerCards.value = false
  }
}

async function confirmPickerAdd() {
  if (!editBanner.value || pickerSelected.value.size === 0) return
  addingPickerCards.value = true
  pageError.value = null
  try {
    const ids = Array.from(pickerSelected.value)
    await gachaAdminApi.addBannerCards(editBanner.value.id, ids)
    // Merge into local pool
    for (const id of ids) {
      if (!bannerCurrentCardIds.value.includes(id)) {
        bannerCurrentCardIds.value.push(id)
      }
    }
    pickerSelected.value = new Set()
    bannerPickerOpen.value = false
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    addingPickerCards.value = false
  }
}

// Reset picker view state when banner dialog closes
watch(showBannerDialog, (open) => {
  if (!open) {
    bannerPickerOpen.value = false
    pickerSearch.value = ''
    pickerSelected.value = new Set()
  }
})

const bannerForm = ref({
  name: '',
  description: '',
  is_standard: false,
  enabled: true,
  active_from: '',
  active_to: '',
  sort_order: 0,
  backdrop_path: '',  // single banner background image (slider/spin-page)
})

function openBannerCreate() {
  editBanner.value = null
  bannerForm.value = { name: '', description: '', is_standard: false, enabled: true, active_from: '', active_to: '', sort_order: 0, backdrop_path: '' }
  bannerBackdropUrl.value = ''
  bannerCurrentCardIds.value = []
  bannerPickerOpen.value = false
  showBannerDialog.value = true
}

async function openBannerEdit(banner: GachaBanner) {
  editBanner.value = banner
  bannerForm.value = {
    name: banner.name,
    description: banner.description ?? '',
    is_standard: banner.is_standard,
    enabled: banner.enabled,
    active_from: banner.active_from ?? '',
    active_to: banner.active_to ?? '',
    sort_order: banner.sort_order ?? 0,
    backdrop_path: banner.backdrop_path ?? '',
  }
  bannerBackdropUrl.value = ''
  bannerCurrentCardIds.value = []
  bannerPickerOpen.value = false
  showBannerDialog.value = true
  // Fetch current pool
  loadingBannerPool.value = true
  try {
    const res = await gachaAdminApi.getBanner(banner.id)
    const detail = (res as { data?: { data?: { card_ids?: string[] } } }).data?.data
    bannerCurrentCardIds.value = detail?.card_ids ?? []
  } catch {
    // non-fatal: pool just stays empty
  } finally {
    loadingBannerPool.value = false
  }
}

async function saveBanner() {
  savingBanner.value = true
  pageError.value = null
  try {
    const payload = {
      name: bannerForm.value.name,
      description: bannerForm.value.description,
      is_standard: bannerForm.value.is_standard,
      enabled: bannerForm.value.enabled,
      active_from: bannerForm.value.active_from || undefined,
      active_to: bannerForm.value.active_to || undefined,
      sort_order: bannerForm.value.sort_order,
      // Carry backdrop_path so editing a banner doesn't wipe it
      // (backend UpdateBanner overwrites the field from the request body).
      backdrop_path: bannerForm.value.backdrop_path,
    }
    if (editBanner.value) {
      await gachaAdminApi.updateBanner(editBanner.value.id, payload)
    } else {
      await gachaAdminApi.createBanner(payload)
    }
    showBannerDialog.value = false
    await loadBanners()
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    savingBanner.value = false
  }
}

// M5: remove a single card from the banner pool via setBannerCards
async function removeBannerCard(cardId: string) {
  if (!editBanner.value) return
  const newPool = bannerCurrentCardIds.value.filter(id => id !== cardId)
  try {
    await gachaAdminApi.setBannerCards(editBanner.value.id, newPool)
    bannerCurrentCardIds.value = newPool
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  }
}

// Helper: card name for display in pool (M5)
function cardNameById(id: string): string {
  return cards.value.find(c => c.id === id)?.name ?? ''
}

function cardById(id: string): GachaCard | undefined {
  return cards.value.find(c => c.id === id)
}

// ── Delete confirm ────────────────────────────────────────────────────────────
const showDeleteDialog = ref(false)
const deleting = ref(false)
const deleteTarget = ref<{ label: string; confirmMsg: string; action: () => Promise<void> } | null>(null)

function confirmCardDelete(card: GachaCard) {
  deleteTarget.value = {
    label: card.name,
    confirmMsg: t('gacha.admin.card_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteCard(card.id)
      await loadCards()
    },
  }
  showDeleteDialog.value = true
}

function confirmGroupDelete(group: GachaGroup) {
  deleteTarget.value = {
    label: group.name,
    confirmMsg: t('gacha.admin.group_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteGroup(group.id)
      await loadGroups()
    },
  }
  showDeleteDialog.value = true
}

function confirmBannerDelete(banner: GachaBanner) {
  deleteTarget.value = {
    label: banner.name,
    confirmMsg: t('gacha.admin.banner_delete_confirm'),
    action: async () => {
      await gachaAdminApi.deleteBanner(banner.id)
      await loadBanners()
    },
  }
  showDeleteDialog.value = true
}

async function runDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  pageError.value = null
  try {
    await deleteTarget.value.action()
    showDeleteDialog.value = false
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    deleting.value = false
  }
}

// ── Data loaders ──────────────────────────────────────────────────────────────
async function loadCards() {
  loadingCards.value = true
  try {
    // M6: server-side group filter
    const params = cardFilterGroup.value !== 'all' ? { group_id: cardFilterGroup.value } : undefined
    const res = await gachaAdminApi.listCards(params)
    cards.value = ((res as { data?: { data?: GachaCard[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingCards.value = false
  }
}

async function loadGroups() {
  loadingGroups.value = true
  try {
    const res = await gachaAdminApi.listGroups()
    groups.value = ((res as { data?: { data?: GachaGroup[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingGroups.value = false
  }
}

async function loadBanners() {
  loadingBanners.value = true
  try {
    const res = await gachaAdminApi.listBanners()
    banners.value = ((res as { data?: { data?: GachaBanner[] } }).data?.data ?? [])
  } catch (err: unknown) {
    pageError.value = extractMessage(err)
  } finally {
    loadingBanners.value = false
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function extractMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

const rarityOptions = [
  { value: 'N', label: 'N' },
  { value: 'R', label: 'R' },
  { value: 'SR', label: 'SR' },
  { value: 'SSR', label: 'SSR' },
]

function rarityBadgeClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'bg-orange-400/20 text-orange-400'
    case 'SR':  return 'bg-indigo-400/20 text-indigo-400'
    case 'R':   return 'bg-teal-400/20 text-teal-400'
    default:    return 'bg-white/10 text-white/60'
  }
}

// M6: re-fetch cards when group filter changes (server-side filter)
watch(cardFilterGroup, () => { void loadCards() })

onMounted(async () => {
  await Promise.all([loadCards(), loadGroups(), loadBanners()])
})
</script>
