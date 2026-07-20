import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Select, MenuItem, FormControl, TextField, Chip, ToggleButton, ToggleButtonGroup,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Checkbox, InputAdornment, Divider,
} from '@mui/material'
import { CloudUpload, Schedule, Search, Delete, Edit, Check, Close } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping } from '../types'

function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
// unix-секунды -> строка для <input datetime-local> (в локальном времени)
function toLocalInput(n: number) {
  const d = new Date(n * 1000)
  const p = (x: number) => String(x).padStart(2, '0')
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

export default function Push() {
  const qc = useQueryClient()
  const [scope, setScope] = useState<'category' | 'products' | 'all_unpushed'>('category')
  const [selectedCat, setSelectedCat] = useState('')
  const [scheduleAt, setScheduleAt] = useState('')
  // выбор товаров
  const [search, setSearch] = useState('')
  const [checked, setChecked] = useState<Record<number, boolean>>({})
  // редактирование времени плана
  const [editId, setEditId] = useState<number | null>(null)
  const [editAt, setEditAt] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['push'],
    queryFn: () => api.get('/push').then(r => r.data.data),
  })
  const mappings: CategoryMapping[] = data?.mappings ?? []
  const settings = data?.settings ?? {}
  const unpushedCount = data?.unpushed_count ?? 0

  // список незапушенных товаров (для режима «выбранные»)
  const { data: prodResp } = useQuery({
    queryKey: ['unpushed', search],
    queryFn: () => api.get('/products', { params: { unpushed: 1, enabled: 1, search, per_page: 200, sort: 'sales' } }).then(r => r.data.data),
    enabled: scope === 'products',
  })
  const products: any[] = prodResp?.products ?? prodResp?.Products ?? []
  const selectedIds = Object.keys(checked).filter(k => checked[+k]).map(Number)

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
      // для товаров/всех — планируем на «сейчас» (тот же механизм)
      return api.post('/sync/schedule', { task_type: 'push', ...targetPayload(), scheduled_at: Math.floor(Date.now() / 1000) + 5, label: targetLabel() })
    },
    onSuccess: () => { toast.success('Push запущен'); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Ошибка push'),
  })

  const schedulePush = useMutation({
    mutationFn: () => api.post('/sync/schedule', {
      task_type: 'push', ...targetPayload(),
      scheduled_at: Math.floor(new Date(scheduleAt).getTime() / 1000), label: targetLabel(),
    }),
    onSuccess: () => { toast.success('Push запланирован'); setScheduleAt(''); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Ошибка планирования'),
  })

  const { data: schedules = [] } = useQuery({
    queryKey: ['schedules'],
    queryFn: () => api.get('/sync/schedules').then(r => r.data.data ?? []),
    refetchInterval: 15_000,
  })
  const pushSchedules = schedules.filter((s: any) => s.task_type === 'push')

  const delSchedule = useMutation({
    mutationFn: (id: number) => api.delete(`/sync/schedule/${id}`),
    onSuccess: () => { toast.success('Удалено'); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Не удалось удалить'),
  })
  const updSchedule = useMutation({
    mutationFn: ({ id, at }: { id: number; at: string }) =>
      api.put(`/sync/schedule/${id}`, { task_type: 'push', scheduled_at: Math.floor(new Date(at).getTime() / 1000) }),
    onSuccess: () => { toast.success('Время изменено'); setEditId(null); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Не удалось изменить'),
  })

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Push в магазин</Typography>
      {isLoading && <LinearProgress />}

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: '4px' }}>Что выгружаем в CS-Cart</Typography>
          <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 2 }}>
            Статус новых товаров: <strong>{settings.default_product_status || 'A'}</strong>. Незапушенных сейчас: <strong>{unpushedCount}</strong>.
          </Typography>

          <ToggleButtonGroup exclusive size="small" value={scope} onChange={(_, v) => v && setScope(v)} sx={{ mb: 2 }}>
            <ToggleButton value="category">По категории</ToggleButton>
            <ToggleButton value="products">Выбранные товары</ToggleButton>
            <ToggleButton value="all_unpushed">Все незапушенные</ToggleButton>
          </ToggleButtonGroup>

          {scope === 'category' && (
            <FormControl size="small" fullWidth sx={{ maxWidth: 560, mb: 2 }}>
              <Select value={selectedCat} displayEmpty onChange={e => setSelectedCat(e.target.value as string)}>
                <MenuItem value=""><em>— выбери категорию —</em></MenuItem>
                {mappings.map(m => (
                  <MenuItem key={m.OTCategoryID} value={m.OTCategoryID}>
                    {m.OTCategoryID} → {m.CSCategoryName}{m.Notes && ` (${m.Notes})`}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          )}

          {scope === 'products' && (
            <Box sx={{ mb: 2 }}>
              <Stack direction="row" spacing={2} sx={{ mb: 1, alignItems: 'center' }}>
                <TextField size="small" placeholder="Поиск товара…" value={search} onChange={e => setSearch(e.target.value)}
                  slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }} sx={{ width: 320 }} />
                <Chip label={`Выбрано: ${selectedIds.length}`} color={selectedIds.length ? 'primary' : 'default'} size="small" />
              </Stack>
              <TableContainer sx={{ maxHeight: 340, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell padding="checkbox"></TableCell>
                      <TableCell>Товар</TableCell>
                      <TableCell align="right">Цена</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {products.map((p: any) => {
                      const id = p.ID ?? p.id
                      return (
                        <TableRow key={id} hover onClick={() => setChecked(c => ({ ...c, [id]: !c[id] }))} sx={{ cursor: 'pointer' }}>
                          <TableCell padding="checkbox"><Checkbox size="small" checked={!!checked[id]} /></TableCell>
                          <TableCell><Typography sx={{ fontSize: 12 }}>{p.TitleRU ?? p.title_ru ?? p.TitleOriginal ?? id}</Typography></TableCell>
                          <TableCell align="right"><Typography sx={{ fontSize: 12 }}>{p.PriceTMT ?? p.price_tmt ?? '-'}</Typography></TableCell>
                        </TableRow>
                      )
                    })}
                    {products.length === 0 && <TableRow><TableCell colSpan={3}><Typography sx={{ fontSize: 12, color: 'text.secondary', p: 1 }}>Нет незапушенных товаров по фильтру</Typography></TableCell></TableRow>}
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          )}

          {scope === 'all_unpushed' && (
            <Typography sx={{ mb: 2, fontSize: 14 }}>Будут выгружены <strong>все {unpushedCount}</strong> незапушенных товаров (по маппингу их категорий).</Typography>
          )}

          <Divider sx={{ my: 2 }} />

          <Stack direction="row" spacing={2} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 2 }}>
            <Button variant="contained" color="success" startIcon={<CloudUpload />}
              disabled={!targetReady() || pushNow.isPending} onClick={() => pushNow.mutate()}>
              Запустить сейчас
            </Button>
            <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap', pl: { sm: 2 }, borderLeft: { sm: '1px solid' }, borderColor: { sm: 'divider' } }}>
              <TextField type="datetime-local" size="small" label="Время запуска (ночь)"
                value={scheduleAt} onChange={e => setScheduleAt(e.target.value)}
                slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 230 }} />
              <Button variant="outlined" startIcon={<Schedule />}
                disabled={!targetReady() || !scheduleAt || schedulePush.isPending} onClick={() => schedulePush.mutate()}>
                Запланировать
              </Button>
            </Box>
          </Stack>
          <Typography sx={{ fontSize: 11, color: 'text.secondary', mt: 1 }}>
            Push долгий (скачивание фото, создание карточек) — удобно ставить на ночь.
          </Typography>
        </CardContent>
      </Card>

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
                    <TableCell>Цель</TableCell>
                    <TableCell>Что</TableCell>
                    <TableCell>Время запуска</TableCell>
                    <TableCell>Статус</TableCell>
                    <TableCell align="right">Действия</TableCell>
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
                            <TextField type="datetime-local" size="small" value={editAt} onChange={e => setEditAt(e.target.value)}
                              slotProps={{ inputLabel: { shrink: true } }} sx={{ width: 200 }} />
                            <Button size="small" onClick={() => updSchedule.mutate({ id: s.id, at: editAt })}><Check fontSize="small" /></Button>
                            <Button size="small" color="inherit" onClick={() => setEditId(null)}><Close fontSize="small" /></Button>
                          </Stack>
                        ) : ts(s.scheduled_at)}
                      </TableCell>
                      <TableCell><Chip size="small" label={schedLabel(s.status)} color={schedColor(s.status)} /></TableCell>
                      <TableCell align="right">
                        {s.status === 'pending' && editId !== s.id && (
                          <Button size="small" startIcon={<Edit fontSize="small" />} onClick={() => { setEditId(s.id); setEditAt(toLocalInput(s.scheduled_at)) }}>
                            Время
                          </Button>
                        )}
                        <Button size="small" color="error" startIcon={<Delete fontSize="small" />}
                          disabled={s.status === 'running'} onClick={() => delSchedule.mutate(s.id)}>
                          Удалить
                        </Button>
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
