import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Select, MenuItem, FormControl, TextField, Chip, ToggleButton, ToggleButtonGroup,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Checkbox, InputAdornment, Divider, Grid, Card as MCard, CardMedia, CardContent as MCardContent,
  ListSubheader,
} from '@mui/material'
import { CloudUpload, Schedule, Search, Delete, Edit, Check, Close, ViewModule, ViewList, DoneAll, ClearAll } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping } from '../types'

function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
function toLocalInput(n: number) {
  const d = new Date(n * 1000); const p = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
function schedLabel(s: string): string {
  return { pending: 'Ожидает', running: 'Выполняется', done: 'Запущен', cancelled: 'Отменён', error: 'Ошибка' }[s] || s
}
function schedColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : s === 'cancelled' ? 'default' : 'warning'
}
function scopeLabel(sc: string): string {
  return { category: 'Категория', products: 'Выбранные товары', all_unpushed: 'Все незапушенные' }[sc] || 'Категория'
}
// статус задачи выгрузки (для панели статуса Шатлыка)
function pushStatusLabel(s: string, errors: number): string {
  if (s === 'done') return errors > 0 ? 'Завершён с ошибками' : 'Завершён'
  return { pending: 'Запущен', running: 'В процессе', error: 'Завершён с ошибкой' }[s] || s
}
function pushStatusColor(s: string, errors: number): any {
  if (s === 'done') return errors > 0 ? 'warning' : 'success'
  return s === 'error' ? 'error' : 'info'
}
function fmtDuration(start?: number | null, end?: number | null): string {
  if (!start) return '—'
  const e = end || Math.floor(Date.now() / 1000)
  const sec = Math.max(0, e - start)
  if (sec < 60) return `${sec} сек`
  const m = Math.floor(sec / 60), s = sec % 60
  return s ? `${m} мин ${s} сек` : `${m} мин`
}
// alicdn (китайский CDN) недоступен/медленный из Туркменистана → гоним картинки
// через наш серверный прокси. ВАЖНО: просим у alicdn thumbnail (суффикс _NxN.jpg) —
// он ~7-17КБ и проходит throttle China-CDN за <0.3с, тогда как полный ~500КБ виснет.
function imgProxy(url?: string, size = 200): string {
  if (!url) return '/placeholder.png'
  if (url.startsWith('/')) return url
  let u = url
  if (/alicdn\.com|taobaocdn|tbcdn/i.test(url) && !/_\d+x\d+\.(jpg|png)/i.test(url)) {
    u = `${url}_${size}x${size}.jpg`
  }
  return '/otweb/img-proxy?url=' + encodeURIComponent(u)
}
// название товара на выбранном языке
function prodName(p: any, lang: 'ru' | 'tk' | 'zh'): string {
  if (lang === 'zh') return p.TitleOriginal || p.TitleRu || String(p.ID)
  if (lang === 'tk') return p.TitleTk || p.TitleRu || p.TitleOriginal || String(p.ID)
  return p.TitleRu || p.TitleTk || p.TitleOriginal || String(p.ID)
}

export default function Push() {
  const qc = useQueryClient()
  const [scope, setScope] = useState<'category' | 'products' | 'all_unpushed'>('category')
  const [selectedCat, setSelectedCat] = useState('')
  const [scheduleAt, setScheduleAt] = useState('')
  // выбор товаров
  const [search, setSearch] = useState('')
  const [prodCat, setProdCat] = useState('')          // фильтр товаров по категории
  const [checked, setChecked] = useState<Record<number, boolean>>({})
  const [view, setView] = useState<'grid' | 'list'>('grid')
  const [lang, setLang] = useState<'ru' | 'tk' | 'zh'>('ru')
  // редактирование времени плана
  const [editId, setEditId] = useState<number | null>(null)
  const [editAt, setEditAt] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['push'],
    queryFn: () => api.get('/push').then(r => r.data.data),
  })
  // задачи выгрузки (статус/статистика/время) — поллинг, пока есть активная
  const { data: pushJobs } = useQuery({
    queryKey: ['pushJobs'],
    queryFn: () => api.get('/push/jobs?limit=8').then(r => r.data.data as any[]),
    refetchInterval: (q) => {
      const jobs = q.state.data as any[] | undefined
      return jobs?.some(j => j.status === 'running' || j.status === 'pending') ? 2000 : 20000
    },
  })
  const jobs: any[] = pushJobs ?? []
  const lastJob = jobs[0]

  const mappings: CategoryMapping[] = data?.mappings ?? []
  const unpushedCount = data?.unpushed_count ?? 0
  const disabledCount = data?.disabled_count ?? 0
  const byCat: Record<string, number> = data?.unpushed_by_category ?? {}

  // dropdown категорий: сначала с непушенными (по убыванию), потом остальные
  const catOptions = [...mappings]
    .map(m => ({ ...m, n: byCat[m.OTCategoryID] || 0 }))
    .sort((a, b) => (b.n - a.n) || a.CSCategoryName.localeCompare(b.CSCategoryName))
  const withUnpushed = catOptions.filter(c => c.n > 0)
  const noUnpushed = catOptions.filter(c => c.n === 0)

  // список незапушенных товаров (для режима «выбранные»)
  const { data: prodResp } = useQuery({
    queryKey: ['unpushed', search, prodCat],
    queryFn: () => api.get('/products', { params: { unpushed: 1, enabled: 1, search, category: prodCat, per_page: 48, sort: 'sales' } }).then(r => r.data.data),
    enabled: scope === 'products',
  })
  const products: any[] = prodResp?.products ?? []
  const selectedIds = Object.keys(checked).filter(k => checked[+k]).map(Number)
  const allShownChecked = products.length > 0 && products.every((p: any) => checked[p.ID])
  const toggleAll = () => {
    if (allShownChecked) { const c = { ...checked }; products.forEach((p: any) => delete c[p.ID]); setChecked(c) }
    else { const c = { ...checked }; products.forEach((p: any) => { c[p.ID] = true }); setChecked(c) }
  }

  const targetPayload = () => {
    if (scope === 'products') return { push_scope: 'products', product_ids: selectedIds }
    if (scope === 'all_unpushed') return { push_scope: 'all_unpushed' }
    return { push_scope: 'category', category_id: selectedCat }
  }
  const targetLabel = () => {
    if (scope === 'products') return `${selectedIds.length} выбранных товаров`
    if (scope === 'all_unpushed') return `Все незапушенные (${unpushedCount})`
    return mappings.find(m => m.OTCategoryID === selectedCat)?.CSCategoryName || selectedCat
  }
  const targetReady = () => scope === 'all_unpushed' || (scope === 'category' && !!selectedCat) || (scope === 'products' && selectedIds.length > 0)

  const pushNow = useMutation({
    mutationFn: () => {
      if (scope === 'category') return api.post('/push/api', { category_id: selectedCat })
      return api.post('/sync/schedule', { task_type: 'push', ...targetPayload(), scheduled_at: Math.floor(Date.now() / 1000) + 5, label: targetLabel() })
    },
    onSuccess: () => { toast.success('Push запущен'); qc.invalidateQueries({ queryKey: ['schedules'] }); qc.invalidateQueries({ queryKey: ['pushJobs'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Ошибка push'),
  })
  const schedulePush = useMutation({
    mutationFn: () => api.post('/sync/schedule', { task_type: 'push', ...targetPayload(), scheduled_at: Math.floor(new Date(scheduleAt).getTime() / 1000), label: targetLabel() }),
    onSuccess: () => { toast.success('Push запланирован'); setScheduleAt(''); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Ошибка планирования'),
  })

  const { data: schedules = [] } = useQuery({
    queryKey: ['schedules'], queryFn: () => api.get('/sync/schedules').then(r => r.data.data ?? []), refetchInterval: 15_000,
  })
  const pushSchedules = schedules.filter((s: any) => s.task_type === 'push')
  const delSchedule = useMutation({
    mutationFn: (id: number) => api.delete(`/sync/schedule/${id}`),
    onSuccess: () => { toast.success('Удалено'); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Не удалось удалить'),
  })
  const updSchedule = useMutation({
    mutationFn: ({ id, at }: { id: number; at: string }) => api.put(`/sync/schedule/${id}`, { task_type: 'push', scheduled_at: Math.floor(new Date(at).getTime() / 1000) }),
    onSuccess: () => { toast.success('Время изменено'); setEditId(null); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Не удалось изменить'),
  })

  const catMenuItem = (c: any) => (
    <MenuItem key={c.OTCategoryID} value={c.OTCategoryID}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', width: '100%', alignItems: 'center', gap: 1 }}>
        <Typography sx={{ fontSize: 13, fontWeight: c.n > 0 ? 600 : 400 }}>
          {c.CSCategoryName}{c.Notes ? ` (${c.Notes})` : ''}
        </Typography>
        {c.n > 0
          ? <Chip size="small" color="success" label={`${c.n} новых`} sx={{ height: 20, fontSize: 11 }} />
          : <Typography sx={{ fontSize: 11, color: 'text.disabled' }}>0</Typography>}
      </Box>
    </MenuItem>
  )

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Push в магазин</Typography>
      {isLoading && <LinearProgress />}

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: '4px' }}>Что выгружаем в CS-Cart</Typography>
          <Stack direction="row" spacing={1} sx={{ mb: 2, flexWrap: 'wrap', gap: 1, alignItems: 'center' }}>
            <Chip size="small" color="success" variant="outlined" label={`Готовы к пушу (включённые): ${unpushedCount}`} />
            <Chip size="small" color="default" variant="outlined" label={`Выключены — в пуш не пойдут: ${disabledCount}`} />
            <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
              Пушим только включённые. Выключенные держатся намеренно (не удалены).
            </Typography>
          </Stack>

          <ToggleButtonGroup exclusive size="small" value={scope} onChange={(_, v) => v && setScope(v)} sx={{ mb: 2 }}>
            <ToggleButton value="category">По категории</ToggleButton>
            <ToggleButton value="products">Выбранные товары</ToggleButton>
            <ToggleButton value="all_unpushed">Все незапушенные</ToggleButton>
          </ToggleButtonGroup>

          {scope === 'category' && (
            <FormControl size="small" fullWidth sx={{ maxWidth: 620, mb: 2 }}>
              <Select value={selectedCat} displayEmpty onChange={e => setSelectedCat(e.target.value as string)}
                renderValue={(v) => v ? (mappings.find(m => m.OTCategoryID === v)?.CSCategoryName || v) : <em>— выбери категорию —</em>}>
                <MenuItem value=""><em>— выбери категорию —</em></MenuItem>
                {withUnpushed.length > 0 && <ListSubheader sx={{ color: 'success.main', fontWeight: 700 }}>Есть непушенные товары</ListSubheader>}
                {withUnpushed.map(catMenuItem)}
                {noUnpushed.length > 0 && <ListSubheader>Всё выгружено</ListSubheader>}
                {noUnpushed.map(catMenuItem)}
              </Select>
            </FormControl>
          )}

          {scope === 'products' && (
            <Box sx={{ mb: 2 }}>
              <Stack direction="row" spacing={1.5} sx={{ mb: 1.5, alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
                <TextField size="small" placeholder="Поиск товара…" value={search} onChange={e => setSearch(e.target.value)}
                  slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }} sx={{ width: 260 }} />
                <FormControl size="small" sx={{ minWidth: 200 }}>
                  <Select value={prodCat} displayEmpty onChange={e => setProdCat(e.target.value as string)}
                    renderValue={(v) => v ? (mappings.find(m => m.OTCategoryID === v)?.CSCategoryName || v) : 'Все категории'}>
                    <MenuItem value="">Все категории</MenuItem>
                    {withUnpushed.map(catMenuItem)}
                  </Select>
                </FormControl>
                {/* язык отображения названий */}
                <ToggleButtonGroup exclusive size="small" value={lang} onChange={(_, v) => v && setLang(v)}>
                  <ToggleButton value="ru">RU</ToggleButton>
                  <ToggleButton value="tk">TK</ToggleButton>
                  <ToggleButton value="zh">中文</ToggleButton>
                </ToggleButtonGroup>
                {/* grid / list */}
                <ToggleButtonGroup exclusive size="small" value={view} onChange={(_, v) => v && setView(v)}>
                  <ToggleButton value="grid"><ViewModule fontSize="small" /></ToggleButton>
                  <ToggleButton value="list"><ViewList fontSize="small" /></ToggleButton>
                </ToggleButtonGroup>
                <Box sx={{ flex: 1 }} />
                <Button size="small" startIcon={allShownChecked ? <ClearAll /> : <DoneAll />} onClick={toggleAll}>
                  {allShownChecked ? 'Снять всё' : 'Выбрать всё'}
                </Button>
                <Chip label={`Выбрано: ${selectedIds.length}`} color={selectedIds.length ? 'primary' : 'default'} size="small" />
              </Stack>

              {view === 'grid' ? (
                <Box sx={{ maxHeight: 460, overflowY: 'auto', border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1 }}>
                  <Grid container spacing={1}>
                    {products.map((p: any) => (
                      <Grid key={p.ID} size={{ xs: 6, sm: 4, md: 3, lg: 2 }}>
                        <MCard variant="outlined" onClick={() => setChecked(c => ({ ...c, [p.ID]: !c[p.ID] }))}
                          sx={{ cursor: 'pointer', position: 'relative', borderColor: checked[p.ID] ? 'primary.main' : 'divider', borderWidth: checked[p.ID] ? 2 : 1 }}>
                          <Checkbox size="small" checked={!!checked[p.ID]} sx={{ position: 'absolute', top: 2, left: 2, bgcolor: 'rgba(255,255,255,.7)', p: 0.3, borderRadius: 1 }} />
                          <CardMedia component="img" loading="lazy" image={imgProxy(p.MainImageURL)} sx={{ aspectRatio: '1/1', objectFit: 'cover', bgcolor: '#f5f5f5' }} />
                          <MCardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                            <Typography sx={{ fontSize: 11, lineHeight: 1.3, height: 42, overflow: 'hidden' }}>{prodName(p, lang)}</Typography>
                            <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'success.main', mt: 0.5 }}>{p.PriceTMT ? `${p.PriceTMT} TMT` : '-'}</Typography>
                          </MCardContent>
                        </MCard>
                      </Grid>
                    ))}
                  </Grid>
                  {products.length === 0 && <Typography sx={{ fontSize: 12, color: 'text.secondary', p: 2, textAlign: 'center' }}>Нет незапушенных товаров по фильтру</Typography>}
                </Box>
              ) : (
                <TableContainer sx={{ maxHeight: 460, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                  <Table size="small" stickyHeader>
                    <TableHead>
                      <TableRow>
                        <TableCell padding="checkbox"><Checkbox size="small" checked={allShownChecked} onChange={toggleAll} /></TableCell>
                        <TableCell>Фото</TableCell>
                        <TableCell>Товар</TableCell>
                        <TableCell align="right">Цена</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {products.map((p: any) => (
                        <TableRow key={p.ID} hover onClick={() => setChecked(c => ({ ...c, [p.ID]: !c[p.ID] }))} sx={{ cursor: 'pointer' }}>
                          <TableCell padding="checkbox"><Checkbox size="small" checked={!!checked[p.ID]} /></TableCell>
                          <TableCell><Box component="img" loading="lazy" src={imgProxy(p.MainImageURL, 120)} sx={{ width: 40, height: 40, objectFit: 'cover', borderRadius: 0.5, bgcolor: '#f5f5f5' }} /></TableCell>
                          <TableCell><Typography sx={{ fontSize: 12 }}>{prodName(p, lang)}</Typography></TableCell>
                          <TableCell align="right"><Typography sx={{ fontSize: 12 }}>{p.PriceTMT ? `${p.PriceTMT} TMT` : '-'}</Typography></TableCell>
                        </TableRow>
                      ))}
                      {products.length === 0 && <TableRow><TableCell colSpan={4}><Typography sx={{ fontSize: 12, color: 'text.secondary', p: 1 }}>Нет незапушенных товаров по фильтру</Typography></TableCell></TableRow>}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </Box>
          )}

          {scope === 'all_unpushed' && (
            <Typography sx={{ mb: 2, fontSize: 14 }}>Будут выгружены <strong>все {unpushedCount}</strong> незапушенных товаров (по маппингу их категорий).</Typography>
          )}

          <Divider sx={{ my: 2 }} />

          <Stack direction="row" spacing={2} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 2 }}>
            <Button variant="contained" color="success" startIcon={<CloudUpload />} disabled={!targetReady() || pushNow.isPending} onClick={() => pushNow.mutate()}>
              Запустить сейчас
            </Button>
            <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap', pl: { sm: 2 }, borderLeft: { sm: '1px solid' }, borderColor: { sm: 'divider' } }}>
              <TextField type="datetime-local" size="small" label="Время запуска (ночь)" value={scheduleAt} onChange={e => setScheduleAt(e.target.value)}
                slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 230 }} />
              <Button variant="outlined" startIcon={<Schedule />} disabled={!targetReady() || !scheduleAt || schedulePush.isPending} onClick={() => schedulePush.mutate()}>
                Запланировать
              </Button>
            </Box>
          </Stack>
          <Typography sx={{ fontSize: 11, color: 'text.secondary', mt: 1 }}>Push долгий (фото, карточки) — удобно ставить на ночь.</Typography>
        </CardContent>
      </Card>

      {/* Статус выполнения выгрузки (запрос Шатлыка): этап, статистика, время, результат */}
      {lastJob && (
        <Card sx={{ mb: 2 }}>
          <CardContent>
            <Stack direction="row" spacing={1} sx={{ mb: 1.5, alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
              <CloudUpload fontSize="small" color="action" />
              <Typography sx={{ fontWeight: 600 }}>Статус выполнения</Typography>
              <Chip size="small" color={pushStatusColor(lastJob.status, lastJob.errors)}
                label={pushStatusLabel(lastJob.status, lastJob.errors)}
                variant={lastJob.status === 'running' || lastJob.status === 'pending' ? 'filled' : 'outlined'} />
              {(lastJob.status === 'running' || lastJob.status === 'pending') && (
                <Typography sx={{ fontSize: 12, color: 'info.main' }}>идёт выгрузка…</Typography>
              )}
            </Stack>
            {(lastJob.status === 'running' || lastJob.status === 'pending') && (
              <LinearProgress sx={{ mb: 1.5 }} />
            )}
            <Grid container spacing={1.5} sx={{ mb: 1 }}>
              <Grid size={{ xs: 6, sm: 3 }}>
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>Цель</Typography>
                <Typography sx={{ fontSize: 13, fontWeight: 600 }}>{lastJob.target || '—'}</Typography>
              </Grid>
              <Grid size={{ xs: 6, sm: 3 }}>
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>Успешно выгружено</Typography>
                <Typography sx={{ fontSize: 13, fontWeight: 600, color: 'success.main' }}>{lastJob.pushed ?? 0}</Typography>
              </Grid>
              <Grid size={{ xs: 6, sm: 3 }}>
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>Ошибок</Typography>
                <Typography sx={{ fontSize: 13, fontWeight: 600, color: (lastJob.errors ?? 0) > 0 ? 'error.main' : 'text.primary' }}>{lastJob.errors ?? 0}</Typography>
              </Grid>
              <Grid size={{ xs: 6, sm: 3 }}>
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>Время выполнения</Typography>
                <Typography sx={{ fontSize: 13, fontWeight: 600 }}>{fmtDuration(lastJob.started_at, lastJob.finished_at)}</Typography>
              </Grid>
            </Grid>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
              <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                Запуск: {ts(lastJob.started_at)} · {lastJob.triggered_by === 'scheduled' ? 'по расписанию' : 'вручную'}
              </Typography>
              <Button size="small" variant="outlined" endIcon={<Search />} target="_blank"
                href="https://wabrum.com/admin.php?dispatch=products.manage">
                Проверить в CS-Cart
              </Button>
            </Stack>
            {lastJob.log && (
              <Box sx={{ mt: 1.5, maxHeight: 180, overflow: 'auto', bgcolor: '#0f172a', color: '#cbd5e1', p: 1.5, borderRadius: 1, fontFamily: 'monospace', fontSize: 11, whiteSpace: 'pre-wrap' }}>
                {lastJob.log}
              </Box>
            )}
            {jobs.length > 1 && (
              <Box sx={{ mt: 1.5 }}>
                <Typography sx={{ fontSize: 11, color: 'text.secondary', mb: 0.5 }}>Недавние выгрузки</Typography>
                <Stack spacing={0.5}>
                  {jobs.slice(1, 6).map((j) => (
                    <Stack key={j.id} direction="row" spacing={1} sx={{ alignItems: 'center', fontSize: 12 }}>
                      <Chip size="small" color={pushStatusColor(j.status, j.errors)} label={pushStatusLabel(j.status, j.errors)} variant="outlined" sx={{ height: 18, fontSize: 10 }} />
                      <Typography sx={{ fontSize: 12 }}>{j.target || '—'}</Typography>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>· {j.pushed ?? 0} шт · {fmtDuration(j.started_at, j.finished_at)} · {ts(j.started_at)}</Typography>
                    </Stack>
                  ))}
                </Stack>
              </Box>
            )}
          </CardContent>
        </Card>
      )}

      {pushSchedules.length > 0 && (
        <Card>
          <CardContent>
            <Stack direction="row" spacing={1} sx={{ mb: 1.5, alignItems: 'center' }}>
              <Schedule fontSize="small" color="action" />
              <Typography sx={{ fontWeight: 600 }}>Запланированные пуши</Typography>
            </Stack>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Цель</TableCell><TableCell>Что</TableCell><TableCell>Время запуска</TableCell><TableCell>Статус</TableCell><TableCell align="right">Действия</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {pushSchedules.map((s: any) => (
                    <TableRow key={s.id} hover>
                      <TableCell><Chip size="small" variant="outlined" label={scopeLabel(s.push_scope)} /></TableCell>
                      <TableCell><Typography sx={{ fontSize: 12 }}>{s.label || s.category_id}</Typography></TableCell>
                      <TableCell>
                        {editId === s.id ? (
                          <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                            <TextField type="datetime-local" size="small" value={editAt} onChange={e => setEditAt(e.target.value)} slotProps={{ inputLabel: { shrink: true } }} sx={{ width: 200 }} />
                            <Button size="small" onClick={() => updSchedule.mutate({ id: s.id, at: editAt })}><Check fontSize="small" /></Button>
                            <Button size="small" color="inherit" onClick={() => setEditId(null)}><Close fontSize="small" /></Button>
                          </Stack>
                        ) : ts(s.scheduled_at)}
                      </TableCell>
                      <TableCell><Chip size="small" label={schedLabel(s.status)} color={schedColor(s.status)} /></TableCell>
                      <TableCell align="right">
                        {s.status === 'pending' && editId !== s.id && (
                          <Button size="small" startIcon={<Edit fontSize="small" />} onClick={() => { setEditId(s.id); setEditAt(toLocalInput(s.scheduled_at)) }}>Время</Button>
                        )}
                        <Button size="small" color="error" startIcon={<Delete fontSize="small" />} disabled={s.status === 'running'} onClick={() => delSchedule.mutate(s.id)}>Удалить</Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </CardContent>
        </Card>
      )}
    </Box>
  )
}
