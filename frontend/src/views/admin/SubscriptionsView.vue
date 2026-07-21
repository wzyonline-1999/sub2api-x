<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <!-- Top Toolbar: Left (search + filters) / Right (actions) -->
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Fuzzy user search + filters (wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- User Search -->
            <div
              class="relative w-full sm:w-64"
              data-filter-user-search
            >
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="filterUserKeyword"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10 pr-8"
                @input="debounceSearchFilterUsers"
                @focus="showFilterUserDropdown = true"
              />
              <button
                v-if="selectedFilterUser"
                @click="clearFilterUser"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                :title="t('common.clear')"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>

              <!-- User Dropdown -->
              <div
                v-if="showFilterUserDropdown && (filterUserResults.length > 0 || filterUserKeyword)"
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
              >
                <div
                  v-if="filterUserLoading"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="filterUserResults.length === 0 && filterUserKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') }}
                </div>
                <button
                  v-for="user in filterUserResults"
                  :key="user.id"
                  type="button"
                  @click="selectFilterUser(user)"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                >
                  <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                  <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                </button>
              </div>
            </div>

            <!-- Filters -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.subscriptions.allStatus')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-48">
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.subscriptions.allGroups')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.platform"
                :options="platformFilterOptions"
                :placeholder="t('admin.subscriptions.allPlatforms')"
                @change="applyFilters"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              @click="loadSubscriptions"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.users.columnSettings')"
              >
                <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                <span class="hidden md:inline">{{ t('admin.users.columnSettings') }}</span>
              </button>
              <!-- Dropdown menu -->
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
              >
                <div class="p-2">
                  <!-- User column mode selection -->
                  <div class="mb-2 border-b border-gray-200 pb-2 dark:border-dark-700">
                    <div class="px-3 py-1 text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.subscriptions.columns.user') }}
                    </div>
                    <button
                      @click="setUserColumnMode('email')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ t('admin.users.columns.email') }}</span>
                      <Icon v-if="userColumnMode === 'email'" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <button
                      @click="setUserColumnMode('username')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                    >
                      <span>{{ t('admin.users.columns.username') }}</span>
                      <Icon v-if="userColumnMode === 'username'" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                  <!-- Other columns toggle -->
                  <button
                    v-for="col in toggleableColumns"
                    :key="col.key"
                    @click="toggleColumn(col.key)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
                  >
                    <span>{{ col.label }}</span>
                    <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                  </button>
                </div>
              </div>
            </div>
            <button
              @click="showGuideModal = true"
              class="btn btn-secondary"
              :title="t('admin.subscriptions.guide.showGuide')"
            >
              <Icon name="questionCircle" size="md" />
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              data-testid="reset-card-records"
              @click="openResetCardRecordsModal"
            >
              <Icon name="clock" size="md" class="mr-2" />
              {{ t('admin.subscriptions.resetCards.recordsAction') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              data-testid="grant-reset-cards"
              @click="openGrantResetCardsModal()"
            >
              <Icon name="gift" size="md" class="mr-2" />
              {{ t('admin.subscriptions.resetCards.grantAction') }}
            </button>
            <button @click="showAssignModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.subscriptions.assignSubscription') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Subscriptions Table -->
      <template #table>
        <DataTable
          :columns="columns"
          :data="subscriptions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ userColumnMode === 'email'
                    ? (row.user?.email?.charAt(0).toUpperCase() || '?')
                    : (row.user?.username?.charAt(0).toUpperCase() || '?')
                  }}
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ userColumnMode === 'email'
                  ? (row.user?.email || t('admin.redeem.userPrefix', { id: row.user_id }))
                  : (row.user?.username || '-')
                }}
              </span>
            </div>
          </template>

          <template #cell-group="{ row }">
            <GroupBadge
              v-if="row.group"
              :name="row.group.name"
              :platform="row.group.platform"
              :subscription-type="row.group.subscription_type"
              :rate-multiplier="row.group.rate_multiplier"
              :show-rate="false"
            />
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[280px] space-y-2">
              <!-- Daily Usage -->
              <div v-if="row.group?.daily_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.daily') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.daily_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.daily_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.daily_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatDailyUsageWindow(row) }}</span>
                </div>
              </div>

              <!-- Weekly Usage -->
              <div v-if="row.group?.weekly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.weekly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.weekly_usage_usd, row.group?.weekly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.weekly_usage_usd, row.group?.weekly_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.weekly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.weekly_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.weekly_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.weekly_window_start, 'weekly') }}</span>
                </div>
              </div>

              <!-- Monthly Usage -->
              <div v-if="row.group?.monthly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.monthly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.monthly_usage_usd, row.group?.monthly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.monthly_usage_usd, row.group?.monthly_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.monthly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.monthly_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.monthly_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.monthly_window_start, 'monthly') }}</span>
                </div>
              </div>

              <!-- No Limits - Unlimited badge -->
              <div
                v-if="
                  !row.group?.daily_limit_usd &&
                  !row.group?.weekly_limit_usd &&
                  !row.group?.monthly_limit_usd
                "
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-3 py-2 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('admin.subscriptions.unlimited') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value }">
            <div v-if="value">
              <span
                class="text-sm"
                :class="
                  isExpiringSoon(value)
                    ? 'text-orange-600 dark:text-orange-400'
                    : 'text-gray-700 dark:text-gray-300'
                "
              >
                {{ formatDateTimeToMinute(value) }}
              </span>
              <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
                {{ getDaysRemaining(value) }} {{ t('admin.subscriptions.daysRemaining') }}
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">{{
              t('admin.subscriptions.noExpiration')
            }}</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active'
                  ? 'badge-success'
                  : value === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`admin.subscriptions.status.${value}`) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-violet-50 hover:text-violet-600 dark:hover:bg-violet-900/20 dark:hover:text-violet-300"
                :data-testid="`grant-reset-cards-${row.id}`"
                @click="openGrantResetCardsModal(row)"
              >
                <Icon name="gift" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetCards.grantShort') }}</span>
              </button>
              <button
                v-if="row.status === 'active' || row.status === 'expired'"
                @click="handleExtend(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.adjust') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleResetQuota(row)"
                :disabled="resettingQuota && resettingSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetQuota') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.revoke') }}</span>
              </button>
              <button
                v-if="row.status === 'revoked'"
                @click="handleRestore(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.restore') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('admin.subscriptions.assignFirstSubscription')"
              :action-text="t('admin.subscriptions.assignSubscription')"
              @action="showAssignModal = true"
            />
          </template>
        </DataTable>
      </template>

      <!-- Pagination -->
      <template #pagination>
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
      </template>
    </TablePageLayout>

    <!-- Grant subscription reset cards -->
    <BaseDialog
      :show="showGrantResetCardsModal"
      :title="t('admin.subscriptions.resetCards.grantTitle')"
      width="normal"
      @close="closeGrantResetCardsModal"
    >
      <form
        id="grant-reset-cards-form"
        class="space-y-5"
        @submit.prevent="handleGrantResetCards"
      >
        <div>
          <label for="reset-card-user-search" class="input-label">
            {{ t('admin.subscriptions.form.user') }}
          </label>
          <div class="relative" data-reset-card-user-search>
            <input
              id="reset-card-user-search"
              v-model="resetCardUserSearchKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              role="combobox"
              aria-autocomplete="list"
              aria-controls="reset-card-user-options"
              :aria-expanded="resetCardUserDropdownOpen"
              @input="debounceSearchResetCardUsers"
              @focus="showResetCardUserDropdown = true"
            />
            <button
              v-if="selectedResetCardUser"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
              :aria-label="t('common.clear')"
              @click="clearResetCardUserSelection"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <div
              v-if="resetCardUserDropdownOpen"
              id="reset-card-user-options"
              role="listbox"
              :aria-label="t('admin.subscriptions.form.user')"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
            >
              <div
                v-if="resetCardUserSearchLoading"
                role="status"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="resetCardUserSearchResults.length === 0 && resetCardUserSearchKeyword"
                role="status"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') }}
              </div>
              <button
                v-for="user in resetCardUserSearchResults"
                :key="user.id"
                type="button"
                role="option"
                :aria-selected="selectedResetCardUser?.id === user.id"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                @click="selectResetCardUser(user)"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
              </button>
            </div>
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') }}</label>
          <Select
            v-model="grantResetCardsForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
            :aria-label="t('admin.subscriptions.form.group')"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <GroupOptionItem
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :description="(option as unknown as GroupOption).description"
                :selected="selected"
              />
            </template>
          </Select>
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') }}</p>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="reset-card-count" class="input-label">
              {{ t('admin.subscriptions.resetCards.form.count') }}
            </label>
            <input
              id="reset-card-count"
              v-model.number="grantResetCardsForm.count"
              type="number"
              min="1"
              max="10000"
              required
              class="input"
            />
          </div>
          <div>
            <label for="reset-card-expires-at" class="input-label">
              {{ t('admin.subscriptions.resetCards.form.expiresAt') }}
            </label>
            <input
              id="reset-card-expires-at"
              v-model="grantResetCardsForm.expires_at_local"
              type="datetime-local"
              class="input"
            />
            <p class="input-hint">
              {{ t('admin.subscriptions.resetCards.form.expiresAtHint') }}
            </p>
          </div>
        </div>

        <div>
          <label for="reset-card-notes" class="input-label">
            {{ t('admin.subscriptions.resetCards.form.notes') }}
          </label>
          <textarea
            id="reset-card-notes"
            v-model="grantResetCardsForm.notes"
            rows="3"
            maxlength="1000"
            class="input resize-y"
            :placeholder="t('admin.subscriptions.resetCards.form.notesPlaceholder')"
          ></textarea>
        </div>

        <div
          class="rounded-lg bg-violet-50 px-4 py-3 text-xs leading-5 text-violet-700 dark:bg-violet-900/20 dark:text-violet-300"
        >
          {{ t('admin.subscriptions.resetCards.grantHint') }}
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeGrantResetCardsModal">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="grant-reset-cards-form"
            class="btn btn-primary"
            :disabled="grantingResetCards"
          >
            <svg
              v-if="grantingResetCards"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              grantingResetCards
                ? t('admin.subscriptions.resetCards.granting')
                : t('admin.subscriptions.resetCards.confirmGrant')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Reset-card grant and usage audit records -->
    <BaseDialog
      :show="showResetCardRecordsModal"
      :title="t('admin.subscriptions.resetCards.recordsTitle')"
      width="extra-wide"
      @close="showResetCardRecordsModal = false"
    >
      <div class="space-y-4" data-testid="reset-card-records-dialog">
        <div
          class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 dark:border-dark-700"
        >
          <div
			class="flex items-center gap-1"
			role="tablist"
			:aria-label="t('admin.subscriptions.resetCards.recordsTitle')"
		  >
            <button
			  id="reset-card-records-tab-grants"
              type="button"
              role="tab"
              data-testid="reset-card-records-tab-grants"
              :aria-selected="resetCardRecordsActiveTab === 'grants'"
			  aria-controls="reset-card-records-panel-grants"
			  :tabindex="resetCardRecordsActiveTab === 'grants' ? 0 : -1"
              :class="[
                'border-b-2 px-4 py-2.5 text-sm font-medium transition-colors',
                resetCardRecordsActiveTab === 'grants'
                  ? 'border-primary-500 text-primary-600 dark:text-primary-300'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
              ]"
              @click="resetCardRecordsActiveTab = 'grants'"
			  @keydown.left.prevent="activateResetCardRecordsTab('usages')"
			  @keydown.right.prevent="activateResetCardRecordsTab('usages')"
			  @keydown.home.prevent="activateResetCardRecordsTab('grants')"
			  @keydown.end.prevent="activateResetCardRecordsTab('usages')"
            >
              {{ t('admin.subscriptions.resetCards.tabs.grants') }}
              <span
                class="ml-1 rounded-full bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >
                {{ resetCardGrants.length }}
              </span>
            </button>
            <button
			  id="reset-card-records-tab-usages"
              type="button"
              role="tab"
              data-testid="reset-card-records-tab-usages"
              :aria-selected="resetCardRecordsActiveTab === 'usages'"
			  aria-controls="reset-card-records-panel-usages"
			  :tabindex="resetCardRecordsActiveTab === 'usages' ? 0 : -1"
              :class="[
                'border-b-2 px-4 py-2.5 text-sm font-medium transition-colors',
                resetCardRecordsActiveTab === 'usages'
                  ? 'border-primary-500 text-primary-600 dark:text-primary-300'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
              ]"
              @click="resetCardRecordsActiveTab = 'usages'"
			  @keydown.left.prevent="activateResetCardRecordsTab('grants')"
			  @keydown.right.prevent="activateResetCardRecordsTab('grants')"
			  @keydown.home.prevent="activateResetCardRecordsTab('grants')"
			  @keydown.end.prevent="activateResetCardRecordsTab('usages')"
            >
              {{ t('admin.subscriptions.resetCards.tabs.usages') }}
              <span
                class="ml-1 rounded-full bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
              >
                {{ resetCardAdminUsages.length }}
              </span>
            </button>
          </div>
          <button
            type="button"
            class="btn btn-secondary btn-sm mb-1"
            :disabled="
              resetCardRecordsActiveTab === 'grants'
                ? resetCardGrantsLoading || resetCardGrantsLoadingMore
                : resetCardUsagesLoading || resetCardUsagesLoadingMore
            "
            data-testid="refresh-reset-card-records"
            @click="refreshActiveResetCardRecords"
          >
            <Icon
              name="refresh"
              size="sm"
              class="mr-1.5"
              :class="
                (resetCardRecordsActiveTab === 'grants' &&
                  (resetCardGrantsLoading || resetCardGrantsLoadingMore)) ||
                (resetCardRecordsActiveTab === 'usages' &&
                  (resetCardUsagesLoading || resetCardUsagesLoadingMore))
                  ? 'animate-spin'
                  : ''
              "
            />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div
		  v-if="resetCardRecordsActiveTab === 'grants'"
		  id="reset-card-records-panel-grants"
		  role="tabpanel"
		  aria-labelledby="reset-card-records-tab-grants"
		  tabindex="0"
		>
          <div
            v-if="resetCardGrantsLoading && resetCardGrants.length === 0"
            class="flex justify-center py-14"
            data-testid="reset-card-grants-loading"
          >
            <div
              class="h-7 w-7 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
            ></div>
          </div>
          <div
            v-else-if="resetCardGrantsError && resetCardGrants.length === 0"
            class="rounded-xl border border-red-200 bg-red-50 px-5 py-8 text-center dark:border-red-900/50 dark:bg-red-900/20"
            role="alert"
            data-testid="reset-card-grants-error"
          >
            <p class="text-sm text-red-700 dark:text-red-300">{{ resetCardGrantsError }}</p>
            <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadResetCardGrants()">
              {{ t('common.retry') }}
            </button>
          </div>
          <div
            v-else-if="resetCardGrants.length === 0"
            class="py-14 text-center"
            data-testid="reset-card-grants-empty"
          >
            <div
              class="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500"
            >
              <Icon name="gift" size="lg" />
            </div>
            <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.subscriptions.resetCards.emptyGrants') }}
            </p>
          </div>
          <div v-else class="space-y-3">
            <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80">
                <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.user') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.group') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.inventory') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.expiresAt') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.status') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.grantedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr
                  v-for="grant in resetCardGrants"
                  :key="grant.id"
                  :data-testid="`reset-card-grant-${grant.id}`"
                  class="text-gray-700 dark:text-gray-300"
                >
                  <td class="whitespace-nowrap px-4 py-3">
                    <p class="font-medium text-gray-900 dark:text-white">
                      {{ grant.user_email || `#${grant.user_id}` }}
                    </p>
                    <p v-if="grant.user_email" class="text-[11px] text-gray-400">#{{ grant.user_id }}</p>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <p class="font-medium text-gray-900 dark:text-white">{{ grant.group_name }}</p>
                    <p class="text-[11px] text-gray-400">#{{ grant.group_id }}</p>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 tabular-nums">
                    <span class="font-semibold text-violet-600 dark:text-violet-300">
                      {{ grant.remaining_count }}
                    </span>
                    <span class="text-gray-400"> / {{ grant.issued_count }}</span>
                    <p class="text-[11px] text-gray-400">
                      {{ t('admin.subscriptions.resetCards.remainingLabel') }}
                    </p>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    {{
                      grant.expires_at
                        ? formatDateTimeToMinute(grant.expires_at)
                        : t('admin.subscriptions.resetCards.neverExpires')
                    }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <span
                      :class="[
                        'rounded-full px-2 py-1 text-xs font-medium',
                        resetCardGrantStatusClass(grant)
                      ]"
                    >
                      {{ resetCardGrantStatusLabel(grant) }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    {{ formatDateTimeToMinute(grant.created_at) }}
                  </td>
                </tr>
              </tbody>
              </table>
            </div>
            <div
              v-if="resetCardGrantsError"
              class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-center dark:border-red-900/50 dark:bg-red-900/20"
              role="alert"
              data-testid="reset-card-grants-load-more-error"
            >
              <p class="text-sm text-red-700 dark:text-red-300">{{ resetCardGrantsError }}</p>
              <button
                type="button"
                class="btn btn-secondary btn-sm mt-2"
                @click="loadResetCardGrants(resetCardGrantsErrorAppend)"
              >
                {{ t('common.retry') }}
              </button>
            </div>
            <div v-else-if="resetCardGrantsHasMore" class="flex justify-center">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                data-testid="load-more-reset-card-grants"
                :disabled="resetCardGrantsLoadingMore"
                @click="loadResetCardGrants(true)"
              >
                {{
                  resetCardGrantsLoadingMore
                    ? t('admin.subscriptions.resetCards.loadingMore')
                    : t('admin.subscriptions.resetCards.loadMore')
                }}
              </button>
            </div>
          </div>
        </div>

        <div
		  v-else
		  id="reset-card-records-panel-usages"
		  role="tabpanel"
		  aria-labelledby="reset-card-records-tab-usages"
		  tabindex="0"
		>
          <div
            v-if="resetCardUsagesLoading && resetCardAdminUsages.length === 0"
            class="flex justify-center py-14"
            data-testid="reset-card-usages-loading"
          >
            <div
              class="h-7 w-7 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
            ></div>
          </div>
          <div
            v-else-if="resetCardUsagesError && resetCardAdminUsages.length === 0"
            class="rounded-xl border border-red-200 bg-red-50 px-5 py-8 text-center dark:border-red-900/50 dark:bg-red-900/20"
            role="alert"
            data-testid="reset-card-usages-error"
          >
            <p class="text-sm text-red-700 dark:text-red-300">{{ resetCardUsagesError }}</p>
            <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadResetCardAdminUsages()">
              {{ t('common.retry') }}
            </button>
          </div>
          <div
            v-else-if="resetCardAdminUsages.length === 0"
            class="py-14 text-center"
            data-testid="reset-card-usages-empty"
          >
            <div
              class="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500"
            >
              <Icon name="clock" size="lg" />
            </div>
            <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.subscriptions.resetCards.emptyUsages') }}
            </p>
          </div>
          <div v-else class="space-y-3">
            <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/80">
                <tr class="text-left text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.user') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.group') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.mode') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.subscription') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.previousUsage') }}</th>
                  <th class="px-4 py-3">{{ t('admin.subscriptions.resetCards.columns.usedAt') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr
                  v-for="usage in resetCardAdminUsages"
                  :key="usage.id"
                  :data-testid="`reset-card-usage-${usage.id}`"
                  class="text-gray-700 dark:text-gray-300"
                >
                  <td class="whitespace-nowrap px-4 py-3">
                    <p class="font-medium text-gray-900 dark:text-white">
                      {{ usage.user_email || `#${usage.user_id}` }}
                    </p>
                    <p v-if="usage.user_email" class="text-[11px] text-gray-400">#{{ usage.user_id }}</p>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <p class="font-medium text-gray-900 dark:text-white">{{ usage.group_name }}</p>
                    <p class="text-[11px] text-gray-400">#{{ usage.group_id }}</p>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    <span
                      class="rounded-full px-2 py-1 text-xs font-medium"
                      :class="
                        usage.mode === 'auto'
                          ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
                          : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
                      "
                    >
                      {{ t(`admin.subscriptions.resetCards.mode.${usage.mode}`) }}
                    </span>
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 tabular-nums">
                    #{{ usage.subscription_id }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3 text-xs tabular-nums">
                    {{ formatAdminResetCardPreviousUsage(usage) }}
                  </td>
                  <td class="whitespace-nowrap px-4 py-3">
                    {{ formatDateTimeToMinute(usage.used_at) }}
                  </td>
                </tr>
              </tbody>
              </table>
            </div>
            <div
              v-if="resetCardUsagesError"
              class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-center dark:border-red-900/50 dark:bg-red-900/20"
              role="alert"
              data-testid="reset-card-usages-load-more-error"
            >
              <p class="text-sm text-red-700 dark:text-red-300">{{ resetCardUsagesError }}</p>
              <button
                type="button"
                class="btn btn-secondary btn-sm mt-2"
                @click="loadResetCardAdminUsages(resetCardUsagesErrorAppend)"
              >
                {{ t('common.retry') }}
              </button>
            </div>
            <div v-else-if="resetCardUsagesHasMore" class="flex justify-center">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                data-testid="load-more-reset-card-usages"
                :disabled="resetCardUsagesLoadingMore"
                @click="loadResetCardAdminUsages(true)"
              >
                {{
                  resetCardUsagesLoadingMore
                    ? t('admin.subscriptions.resetCards.loadingMore')
                    : t('admin.subscriptions.resetCards.loadMore')
                }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <button type="button" class="btn btn-secondary" @click="showResetCardRecordsModal = false">
          {{ t('common.close') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Assign Subscription Modal -->
    <BaseDialog
      :show="showAssignModal"
      :title="t('admin.subscriptions.assignSubscription')"
      width="normal"
      @close="closeAssignModal"
    >
      <form
        id="assign-subscription-form"
        @submit.prevent="handleAssignSubscription"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.user') }}</label>
          <div class="relative" data-assign-user-search>
            <input
              v-model="userSearchKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              @input="debounceSearchUsers"
              @focus="showUserDropdown = true"
            />
            <button
              v-if="selectedUser"
              @click="clearUserSelection"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <!-- User Dropdown -->
            <div
              v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
            >
              <div
                v-if="userSearchLoading"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') }}
              </div>
              <button
                v-for="user in userSearchResults"
                :key="user.id"
                type="button"
                @click="selectUser(user)"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') }}</label>
          <Select
            v-model="assignForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <GroupOptionItem
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :description="(option as unknown as GroupOption).description"
                :selected="selected"
              />
            </template>
          </Select>
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') }}</label>
          <input v-model.number="assignForm.validity_days" type="number" min="1" class="input" />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAssignModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="assign-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Adjust Subscription Modal -->
    <BaseDialog
      :show="showExtendModal"
      :title="t('admin.subscriptions.adjustSubscription')"
      width="narrow"
      @close="closeExtendModal"
    >
      <form
        v-if="extendingSubscription"
        id="extend-subscription-form"
        @submit.prevent="handleExtendSubscription"
        class="space-y-5"
      >
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.adjustingFor') }}
            <span class="font-medium text-gray-900 dark:text-white">{{
              extendingSubscription.user?.email
            }}</span>
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentExpiration') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{
                extendingSubscription.expires_at
                  ? formatDateTimeToMinute(extendingSubscription.expires_at)
                  : t('admin.subscriptions.noExpiration')
              }}
            </span>
          </p>
          <p v-if="extendingSubscription.expires_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.remainingDays') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ getDaysRemaining(extendingSubscription.expires_at) ?? 0 }}
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.adjustDays') }}</label>
          <div class="flex items-center gap-2">
            <input
              v-model.number="extendForm.days"
              type="number"
              required
              class="input text-center"
              :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.adjustHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div v-if="extendingSubscription" class="flex justify-end gap-3">
          <button @click="closeExtendModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="extend-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.adjusting') : t('admin.subscriptions.adjust') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="showRevokeDialog = false"
    />

    <!-- Restore Confirmation Dialog -->
    <ConfirmDialog
      :show="showRestoreDialog"
      :title="t('admin.subscriptions.restoreSubscription')"
      :message="t('admin.subscriptions.restoreConfirm', { user: restoringSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.restore')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmRestore"
      @cancel="showRestoreDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaConfirm"
      :title="t('admin.subscriptions.resetQuotaTitle')"
      :message="t('admin.subscriptions.resetQuotaConfirm', { user: resettingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.resetQuota')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmResetQuota"
      @cancel="showResetQuotaConfirm = false"
    />
    <!-- Subscription Guide Modal -->
    <teleport to="body">
      <transition name="modal">
        <div v-if="showGuideModal" class="fixed inset-0 z-50 flex items-center justify-center p-4" @mousedown.self="showGuideModal = false">
          <div class="fixed inset-0 bg-black/50" @click="showGuideModal = false"></div>
          <div class="relative max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button type="button" class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="showGuideModal = false">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.subscriptions.guide.title') }}</h2>
            <p class="mb-5 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.guide.subtitle') }}</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.subscriptions.guide.step1.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step1.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line3') }}</li>
              </ol>
              <div class="ml-8 mt-2">
                <router-link
                  to="/admin/groups"
                  @click="showGuideModal = false"
                  class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  {{ t('admin.subscriptions.guide.step1.link') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.subscriptions.guide.step2.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step2.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line3') }}</li>
              </ol>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.subscriptions.guide.step3.title') }}
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in guideActionRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.action }}</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ row.desc }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Tip -->
            <div class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
              {{ t('admin.subscriptions.guide.tip') }}
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showGuideModal = false">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  Group,
  GroupPlatform,
  SubscriptionResetCardGrant,
  SubscriptionResetCardUsage,
  SubscriptionType,
  UserSubscription
} from '@/types'
import type { SimpleUser } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import { formatDateTimeToMinute } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Icon from '@/components/icons/Icon.vue'
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts } from '@/utils/subscriptionQuota'

const { t } = useI18n()
const appStore = useAppStore()

interface GroupOption {
  value: number
  label: string
  description: string | null
  platform: GroupPlatform
  subscriptionType: SubscriptionType
  rate: number
}

// Guide modal state
const showGuideModal = ref(false)

const guideActionRows = computed(() => [
  { action: t('admin.subscriptions.guide.actions.adjust'), desc: t('admin.subscriptions.guide.actions.adjustDesc') },
  { action: t('admin.subscriptions.guide.actions.resetQuota'), desc: t('admin.subscriptions.guide.actions.resetQuotaDesc') },
  { action: t('admin.subscriptions.guide.actions.revoke'), desc: t('admin.subscriptions.guide.actions.revokeDesc') }
])

// User column display mode: 'email' or 'username'
const userColumnMode = ref<'email' | 'username'>('email')
const USER_COLUMN_MODE_KEY = 'subscription-user-column-mode'

const loadUserColumnMode = () => {
  try {
    const saved = localStorage.getItem(USER_COLUMN_MODE_KEY)
    if (saved === 'email' || saved === 'username') {
      userColumnMode.value = saved
    }
  } catch (e) {
    console.error('Failed to load user column mode:', e)
  }
}

const saveUserColumnMode = () => {
  try {
    localStorage.setItem(USER_COLUMN_MODE_KEY, userColumnMode.value)
  } catch (e) {
    console.error('Failed to save user column mode:', e)
  }
}

const setUserColumnMode = (mode: 'email' | 'username') => {
  userColumnMode.value = mode
  saveUserColumnMode()
}

// All available columns
const allColumns = computed<Column[]>(() => [
  {
    key: 'user',
    label: userColumnMode.value === 'email'
      ? t('admin.subscriptions.columns.user')
      : t('admin.users.columns.username'),
    sortable: false
  },
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: false },
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false },
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true },
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false }
])

// Columns that can be toggled (exclude user and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'user' && col.key !== 'actions')
)

// Hidden columns set
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns
const DEFAULT_HIDDEN_COLUMNS: string[] = []

// localStorage key
const HIDDEN_COLUMNS_KEY = 'subscription-hidden-columns'

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    }
  } catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  }
}

// Save column settings to localStorage
const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  } catch (e) {
    console.error('Failed to save columns:', e)
  }
}

// Toggle column visibility
const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

// Check if column is visible
const isColumnVisible = (key: string) => !hiddenColumns.has(key)

// Filtered columns for display
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'user' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

// Column dropdown state
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') }
])

const subscriptions = ref<UserSubscription[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
let abortController: AbortController | null = null

// Toolbar user filter (fuzzy search -> select user_id)
const filterUserKeyword = ref('')
const filterUserResults = ref<SimpleUser[]>([])
const filterUserLoading = ref(false)
const showFilterUserDropdown = ref(false)
const selectedFilterUser = ref<SimpleUser | null>(null)
let filterUserSearchTimeout: ReturnType<typeof setTimeout> | null = null

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

const filters = reactive({
  status: 'active',
  group_id: '',
  platform: '',
  user_id: null as number | null
})

// Sorting state
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const showAssignModal = ref(false)
const showGrantResetCardsModal = ref(false)
const showResetCardRecordsModal = ref(false)
const resetCardRecordsActiveTab = ref<'grants' | 'usages'>('grants')
const showExtendModal = ref(false)
const showRevokeDialog = ref(false)
const showRestoreDialog = ref(false)
const showResetQuotaConfirm = ref(false)
const submitting = ref(false)
const resettingSubscription = ref<UserSubscription | null>(null)
const resettingQuota = ref(false)
const grantingResetCards = ref(false)
const grantResetCardsIdempotencyKey = ref('')
const grantResetCardsIdempotencySignature = ref('')
const resetCardGrants = ref<SubscriptionResetCardGrant[]>([])
const resetCardAdminUsages = ref<SubscriptionResetCardUsage[]>([])
const resetCardGrantsLoading = ref(false)
const resetCardUsagesLoading = ref(false)
const resetCardGrantsLoadingMore = ref(false)
const resetCardUsagesLoadingMore = ref(false)
const resetCardGrantsHasMore = ref(false)
const resetCardUsagesHasMore = ref(false)
const resetCardGrantsError = ref('')
const resetCardUsagesError = ref('')
const resetCardGrantsErrorAppend = ref(false)
const resetCardUsagesErrorAppend = ref(false)
let resetCardGrantsLoadSequence = 0
let resetCardUsagesLoadSequence = 0

const RESET_CARD_RECORDS_PAGE_SIZE = 100

const resetCardErrorMessage = (error: any, fallback: string): string =>
  error?.response?.data?.detail || error?.message || fallback
const extendingSubscription = ref<UserSubscription | null>(null)
const revokingSubscription = ref<UserSubscription | null>(null)
const restoringSubscription = ref<UserSubscription | null>(null)

const assignForm = reactive({
  user_id: null as number | null,
  group_id: null as number | null,
  validity_days: 30
})

const extendForm = reactive({
  days: 30
})

const grantResetCardsForm = reactive({
  user_id: null as number | null,
  group_id: null as number | null,
  count: 1,
  expires_at_local: '',
  notes: ''
})

const resetCardUserSearchKeyword = ref('')
const resetCardUserSearchResults = ref<SimpleUser[]>([])
const resetCardUserSearchLoading = ref(false)
const showResetCardUserDropdown = ref(false)
const selectedResetCardUser = ref<SimpleUser | null>(null)
let resetCardUserSearchTimeout: ReturnType<typeof setTimeout> | null = null
let resetCardUserSearchSequence = 0
const resetCardUserDropdownOpen = computed(
  () =>
    showResetCardUserDropdown.value &&
    (resetCardUserSearchResults.value.length > 0 || !!resetCardUserSearchKeyword.value)
)

// Group options for filter (all groups)
const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') },
  ...groups.value.map((g) => ({ value: g.id.toString(), label: g.name }))
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' }
])

// Group options for assign (only subscription type groups)
const subscriptionGroupOptions = computed(() =>
  groups.value
    .filter((g) => g.subscription_type === 'subscription' && g.status === 'active')
    .map((g) => ({
      value: g.id,
      label: g.name,
      description: g.description,
      platform: g.platform,
      subscriptionType: g.subscription_type,
      rate: g.rate_multiplier
    }))
)

const applyFilters = () => {
  pagination.page = 1
  loadSubscriptions()
}

const loadSubscriptions = async () => {
  if (abortController) {
    abortController.abort()
  }
  const requestController = new AbortController()
  abortController = requestController
  const { signal } = requestController

  loading.value = true
  try {
    const response = await adminAPI.subscriptions.list(
      pagination.page,
      pagination.page_size,
      {
        status: (filters.status as any) || undefined,
        group_id: filters.group_id ? parseInt(filters.group_id) : undefined,
        platform: filters.platform || undefined,
        user_id: filters.user_id || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      {
        signal
      }
    )
    if (signal.aborted || abortController !== requestController) return
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    }
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  } finally {
    if (abortController === requestController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  }
}

// Toolbar user filter search with debounce
const debounceSearchFilterUsers = () => {
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  filterUserSearchTimeout = setTimeout(searchFilterUsers, 300)
}

const searchFilterUsers = async () => {
  const keyword = filterUserKeyword.value.trim()

  // Clear active user filter if user modified the search keyword
  if (selectedFilterUser.value && keyword !== selectedFilterUser.value.email) {
    selectedFilterUser.value = null
    filters.user_id = null
    applyFilters()
  }

  if (!keyword) {
    filterUserResults.value = []
    return
  }

  filterUserLoading.value = true
  try {
    filterUserResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    filterUserResults.value = []
  } finally {
    filterUserLoading.value = false
  }
}

const selectFilterUser = (user: SimpleUser) => {
  selectedFilterUser.value = user
  filterUserKeyword.value = user.email
  showFilterUserDropdown.value = false
  filters.user_id = user.id
  applyFilters()
}

const clearFilterUser = () => {
  selectedFilterUser.value = null
  filterUserKeyword.value = ''
  filterUserResults.value = []
  showFilterUserDropdown.value = false
  filters.user_id = null
  applyFilters()
}

// User search with debounce
const debounceSearchUsers = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  userSearchTimeout = setTimeout(searchUsers, 300)
}

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()

  // Clear selection if user modified the search keyword
  if (selectedUser.value && keyword !== selectedUser.value.email) {
    selectedUser.value = null
    assignForm.user_id = null
  }

  if (!keyword) {
    userSearchResults.value = []
    return
  }

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  } finally {
    userSearchLoading.value = false
  }
}

const selectUser = (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  assignForm.user_id = user.id
}

const clearUserSelection = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  assignForm.user_id = null
}

const appendUniqueResetCardRecords = <T extends { id: number }>(current: T[], incoming: T[]): T[] => {
  const loadedIDs = new Set(current.map((item) => item.id))
  return [...current, ...incoming.filter((item) => !loadedIDs.has(item.id))]
}

const loadResetCardGrants = async (append = false, force = false) => {
  if (!force && (resetCardGrantsLoading.value || resetCardGrantsLoadingMore.value)) return

  const loadSequence = ++resetCardGrantsLoadSequence
  const offset = append ? resetCardGrants.value.length : 0
  resetCardGrantsLoading.value = !append
  resetCardGrantsLoadingMore.value = append
  resetCardGrantsError.value = ''
  resetCardGrantsErrorAppend.value = append
  try {
    const items = await adminAPI.subscriptionResetCards.listGrants({
      limit: RESET_CARD_RECORDS_PAGE_SIZE,
      offset
    })
    if (loadSequence === resetCardGrantsLoadSequence) {
      resetCardGrants.value = append
        ? appendUniqueResetCardRecords(resetCardGrants.value, items)
        : items
      resetCardGrantsHasMore.value = items.length === RESET_CARD_RECORDS_PAGE_SIZE
    }
  } catch (error: any) {
    if (loadSequence === resetCardGrantsLoadSequence) {
      resetCardGrantsError.value =
        resetCardErrorMessage(error, t('admin.subscriptions.resetCards.loadGrantsFailed'))
    }
  } finally {
    if (loadSequence === resetCardGrantsLoadSequence) {
      resetCardGrantsLoading.value = false
      resetCardGrantsLoadingMore.value = false
    }
  }
}

const loadResetCardAdminUsages = async (append = false, force = false) => {
  if (!force && (resetCardUsagesLoading.value || resetCardUsagesLoadingMore.value)) return

  const loadSequence = ++resetCardUsagesLoadSequence
  const offset = append ? resetCardAdminUsages.value.length : 0
  resetCardUsagesLoading.value = !append
  resetCardUsagesLoadingMore.value = append
  resetCardUsagesError.value = ''
  resetCardUsagesErrorAppend.value = append
  try {
    const items = await adminAPI.subscriptionResetCards.listUsages({
      limit: RESET_CARD_RECORDS_PAGE_SIZE,
      offset
    })
    if (loadSequence === resetCardUsagesLoadSequence) {
      resetCardAdminUsages.value = append
        ? appendUniqueResetCardRecords(resetCardAdminUsages.value, items)
        : items
      resetCardUsagesHasMore.value = items.length === RESET_CARD_RECORDS_PAGE_SIZE
    }
  } catch (error: any) {
    if (loadSequence === resetCardUsagesLoadSequence) {
      resetCardUsagesError.value =
        resetCardErrorMessage(error, t('admin.subscriptions.resetCards.loadUsagesFailed'))
    }
  } finally {
    if (loadSequence === resetCardUsagesLoadSequence) {
      resetCardUsagesLoading.value = false
      resetCardUsagesLoadingMore.value = false
    }
  }
}

const openResetCardRecordsModal = () => {
  resetCardRecordsActiveTab.value = 'grants'
  showResetCardRecordsModal.value = true
  void Promise.all([loadResetCardGrants(), loadResetCardAdminUsages()])
}

const activateResetCardRecordsTab = (tab: 'grants' | 'usages') => {
	resetCardRecordsActiveTab.value = tab
	void nextTick(() => {
		document.getElementById(`reset-card-records-tab-${tab}`)?.focus()
	})
}

const refreshActiveResetCardRecords = () => {
  if (resetCardRecordsActiveTab.value === 'grants') {
    void loadResetCardGrants(false)
    return
  }
  void loadResetCardAdminUsages(false)
}

const resetCardGrantDisplayStatus = (grant: SubscriptionResetCardGrant): string => {
  if (grant.remaining_count <= 0 || grant.status === 'exhausted') return 'exhausted'
  if (grant.expires_at && new Date(grant.expires_at) <= new Date()) return 'expired'
  return grant.status
}

const resetCardGrantStatusLabel = (grant: SubscriptionResetCardGrant): string => {
  const status = resetCardGrantDisplayStatus(grant)
  const knownStatuses = ['active', 'exhausted', 'expired', 'revoked']
  return knownStatuses.includes(status)
    ? t(`admin.subscriptions.resetCards.status.${status}`)
    : status
}

const resetCardGrantStatusClass = (grant: SubscriptionResetCardGrant): string => {
  switch (resetCardGrantDisplayStatus(grant)) {
    case 'active':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    case 'exhausted':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
    case 'expired':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
    default:
      return 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
  }
}

const formatAdminResetCardPreviousUsage = (usage: SubscriptionResetCardUsage): string =>
  t('admin.subscriptions.resetCards.previousUsage', {
    daily: usage.previous_daily_usage_usd.toFixed(2),
    weekly: usage.previous_weekly_usage_usd.toFixed(2),
    monthly: usage.previous_monthly_usage_usd.toFixed(2)
  })

const createResetCardGrantIdempotencyKey = () => {
  const requestID =
    globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `subscription-reset-card-grant-${requestID}`
}

const openGrantResetCardsModal = (subscription?: UserSubscription) => {
	resetCardUserSearchSequence++
  grantResetCardsForm.user_id = subscription?.user_id ?? null
  grantResetCardsForm.group_id = subscription?.group_id ?? null
  grantResetCardsForm.count = 1
  grantResetCardsForm.expires_at_local = ''
  grantResetCardsForm.notes = ''
  resetCardUserSearchResults.value = []
  showResetCardUserDropdown.value = false

  if (subscription?.user?.email) {
    selectedResetCardUser.value = {
      id: subscription.user_id,
      email: subscription.user.email,
      deleted: false
    }
    resetCardUserSearchKeyword.value = subscription.user.email
  } else {
    selectedResetCardUser.value = null
    resetCardUserSearchKeyword.value = ''
  }

  grantResetCardsIdempotencyKey.value = ''
  grantResetCardsIdempotencySignature.value = ''
  showGrantResetCardsModal.value = true
}

const closeGrantResetCardsModal = () => {
  if (grantingResetCards.value) return
  showGrantResetCardsModal.value = false
  selectedResetCardUser.value = null
  resetCardUserSearchKeyword.value = ''
  resetCardUserSearchResults.value = []
  showResetCardUserDropdown.value = false
  grantResetCardsForm.user_id = null
  grantResetCardsForm.group_id = null
  grantResetCardsForm.count = 1
  grantResetCardsForm.expires_at_local = ''
  grantResetCardsForm.notes = ''
  grantResetCardsIdempotencyKey.value = ''
  grantResetCardsIdempotencySignature.value = ''
  resetCardUserSearchSequence++
  resetCardUserSearchLoading.value = false
}

const debounceSearchResetCardUsers = () => {
	const sequence = ++resetCardUserSearchSequence
	const keyword = resetCardUserSearchKeyword.value.trim()
	if (selectedResetCardUser.value && keyword !== selectedResetCardUser.value.email) {
		selectedResetCardUser.value = null
		grantResetCardsForm.user_id = null
	}
  if (resetCardUserSearchTimeout) {
    clearTimeout(resetCardUserSearchTimeout)
  }
	if (!keyword) {
		resetCardUserSearchResults.value = []
		resetCardUserSearchLoading.value = false
		return
	}
	resetCardUserSearchTimeout = setTimeout(() => {
		void searchResetCardUsers(sequence)
	}, 300)
}

const searchResetCardUsers = async (sequence: number) => {
	if (sequence !== resetCardUserSearchSequence) return
	const keyword = resetCardUserSearchKeyword.value.trim()

  if (!keyword) {
    resetCardUserSearchResults.value = []
    return
  }

  resetCardUserSearchLoading.value = true
  try {
		const results = await adminAPI.usage.searchUsers(keyword)
		if (sequence === resetCardUserSearchSequence) {
			resetCardUserSearchResults.value = results
		}
  } catch (error) {
		if (sequence === resetCardUserSearchSequence) {
			console.error('Failed to search users for reset-card grant:', error)
			resetCardUserSearchResults.value = []
		}
  } finally {
		if (sequence === resetCardUserSearchSequence) {
			resetCardUserSearchLoading.value = false
		}
  }
}

const selectResetCardUser = (user: SimpleUser) => {
	resetCardUserSearchSequence++
  selectedResetCardUser.value = user
  resetCardUserSearchKeyword.value = user.email
  showResetCardUserDropdown.value = false
  grantResetCardsForm.user_id = user.id
}

const clearResetCardUserSelection = () => {
	resetCardUserSearchSequence++
  selectedResetCardUser.value = null
  resetCardUserSearchKeyword.value = ''
  resetCardUserSearchResults.value = []
  showResetCardUserDropdown.value = false
  grantResetCardsForm.user_id = null
}

const handleGrantResetCards = async () => {
	if (
		!grantResetCardsForm.user_id ||
		!selectedResetCardUser.value ||
		selectedResetCardUser.value.id !== grantResetCardsForm.user_id ||
		resetCardUserSearchKeyword.value.trim() !== selectedResetCardUser.value.email
	) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  }
  if (!grantResetCardsForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  }
	if (
		!subscriptionGroupOptions.value.some(
			(option) => option.value === grantResetCardsForm.group_id
		)
	) {
		appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
		return
	}
  if (
    !Number.isInteger(grantResetCardsForm.count) ||
    grantResetCardsForm.count < 1 ||
    grantResetCardsForm.count > 10000
  ) {
    appStore.showError(t('admin.subscriptions.resetCards.invalidCount'))
    return
  }

  let expiresAt: string | undefined
  if (grantResetCardsForm.expires_at_local) {
    const parsedExpiresAt = new Date(grantResetCardsForm.expires_at_local)
    if (Number.isNaN(parsedExpiresAt.getTime()) || parsedExpiresAt <= new Date()) {
      appStore.showError(t('admin.subscriptions.resetCards.invalidExpiration'))
      return
    }
    expiresAt = parsedExpiresAt.toISOString()
  }

  const payload = {
    user_id: grantResetCardsForm.user_id,
    group_id: grantResetCardsForm.group_id,
    count: grantResetCardsForm.count,
    expires_at: expiresAt,
    notes: grantResetCardsForm.notes.trim() || undefined
  }
  const payloadSignature = JSON.stringify(payload)
  if (
    !grantResetCardsIdempotencyKey.value ||
    grantResetCardsIdempotencySignature.value !== payloadSignature
  ) {
    grantResetCardsIdempotencyKey.value = createResetCardGrantIdempotencyKey()
    grantResetCardsIdempotencySignature.value = payloadSignature
  }

  grantingResetCards.value = true
  try {
    await adminAPI.subscriptionResetCards.grant(
      payload,
      grantResetCardsIdempotencyKey.value
    )
    await loadResetCardGrants(false, true)
    appStore.showSuccess(
      t('admin.subscriptions.resetCards.grantSuccess', {
        count: grantResetCardsForm.count,
        user: selectedResetCardUser.value?.email || `#${grantResetCardsForm.user_id}`
      })
    )
    grantingResetCards.value = false
    closeGrantResetCardsModal()
  } catch (error: any) {
    appStore.showError(resetCardErrorMessage(error, t('admin.subscriptions.resetCards.grantFailed')))
    console.error('Error granting subscription reset cards:', error)
  } finally {
    grantingResetCards.value = false
  }
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSubscriptions()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSubscriptions()
}

const closeAssignModal = () => {
  showAssignModal.value = false
  assignForm.user_id = null
  assignForm.group_id = null
  assignForm.validity_days = 30
  // Clear user search state
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  showUserDropdown.value = false
}

const handleAssignSubscription = async () => {
  if (!assignForm.user_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  }
  if (!assignForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  }
  if (!assignForm.validity_days || assignForm.validity_days < 1) {
    appStore.showError(t('admin.subscriptions.validityDaysRequired'))
    return
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign({
      user_id: assignForm.user_id,
      group_id: assignForm.group_id,
      validity_days: assignForm.validity_days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  showExtendModal.value = true
}

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
}

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  // 前端验证：调整后的过期时间必须在未来
  if (extendingSubscription.value.expires_at) {
    const expiresAt = new Date(extendingSubscription.value.expires_at)
    const newExpiresAt = new Date(expiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000)
    if (newExpiresAt <= new Date()) {
      appStore.showError(t('admin.subscriptions.adjustWouldExpire'))
      return
    }
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, {
      days: extendForm.days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAdjusted'))
    closeExtendModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAdjust'))
    console.error('Error adjusting subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
}

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  }
}

const handleRestore = (subscription: UserSubscription) => {
  restoringSubscription.value = subscription
  showRestoreDialog.value = true
}

const confirmRestore = async () => {
  if (!restoringSubscription.value) return

  try {
    await adminAPI.subscriptions.restore(restoringSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRestored'))
    showRestoreDialog.value = false
    restoringSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRestore'))
    console.error('Error restoring subscription:', error)
  }
}

const handleResetQuota = (subscription: UserSubscription) => {
  resettingSubscription.value = subscription
  showResetQuotaConfirm.value = true
}

const confirmResetQuota = async () => {
  if (!resettingSubscription.value) return
  if (resettingQuota.value) return
  resettingQuota.value = true
  try {
    await adminAPI.subscriptions.resetQuota(resettingSubscription.value.id, { daily: true, weekly: true, monthly: true })
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    showResetQuotaConfirm.value = false
    resettingSubscription.value = null
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToResetQuota'))
    console.error('Error resetting quota:', error)
  } finally {
    resettingQuota.value = false
  }
}

// Helper functions
const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
}

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentage}%`
}

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

const formatResetDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes })
}

const formatQuotaEndDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.quotaEndsInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.quotaEndsInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.quotaEndsInMinutes', { minutes: parts.minutes })
}

const formatDailyUsageWindow = (subscription: UserSubscription): string => {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    return parts ? formatQuotaEndDuration(parts) : t('admin.subscriptions.windowNotActive')
  }

  return formatResetTime(subscription.daily_window_start, 'daily')
}

// Format reset time based on window start and period type
const formatResetTime = (windowStart: string | null, period: 'daily' | 'weekly' | 'monthly'): string => {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')

  const start = new Date(windowStart)
  const now = new Date()

  // Calculate reset time based on period
  let resetTime: Date
  switch (period) {
    case 'daily':
      resetTime = new Date(start.getTime() + 24 * 60 * 60 * 1000)
      break
    case 'weekly':
      resetTime = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000)
      break
    case 'monthly':
      resetTime = new Date(start.getTime() + 30 * 24 * 60 * 60 * 1000)
      break
  }

  const parts = getRemainingDurationParts(resetTime, now)

  return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
}

// Handle click outside to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('[data-assign-user-search]')) showUserDropdown.value = false
  if (!target.closest('[data-reset-card-user-search]')) showResetCardUserDropdown.value = false
  if (!target.closest('[data-filter-user-search]')) showFilterUserDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

onMounted(() => {
  loadUserColumnMode()
  loadSavedColumns()
  loadSubscriptions()
  loadGroups()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  if (resetCardUserSearchTimeout) {
    clearTimeout(resetCardUserSearchTimeout)
  }
})
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
}

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
}
</style>
