import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Select, MenuItem, FormControl, TextField, Chip,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
} from '@mui/material'
import { CloudUpload, Schedule } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { CategoryMapping } from '../types'

function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
function schedLabel(s: string): string {
  return { pending: 'Ожидает', running: 'Запускается', done: 'Запущен', cancelled: 'Отменён', error: 'Ошибка' }[s] || s
}
function schedColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : s === 'cancelled' ? 'default' : 'warning'
}

export default function Push() {
  const [selectedCat, setSelectedCat] = useState('')
  const [scheduleAt, setScheduleAt] = useState('')
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['push'],
    queryFn: () => api.get('/push').then(r => r.data.data),
  })

  const mappings: CategoryMapping[] = data?.mappings ?? []
  const settings = data?.settings ?? {}

  const pushMutation = useMutation({
    mutationFn: (catId: string) => api.post('/push/api', { category_id: catId }),
    onSuccess: () => toast.success('Push запущен'),
    onError: () => toast.error('Ошибка push'),
  })

  // Запланированные пуши (ночной крон)
  const { data: schedules = [] } = useQuery({
    queryKey: ['schedules'],
    queryFn: () => api.get('/sync/schedules').then(r => r.data.data ?? []),
    refetchInterval: 15_000,
  })
  const pushSchedules = schedules.filter((s: any) => s.task_type === 'push')

  const schedulePush = useMutation({
    mutationFn: () => {
      const label = mappings.find(m => m.OTCategoryID === selectedCat)?.CSCategoryName || selectedCat
      return api.post('/sync/schedule', {
        task_type: 'push',
        category_id: selectedCat,
        scheduled_at: Math.floor(new Date(scheduleAt).getTime() / 1000),
        label,
      })
    },
    onSuccess: () => { toast.success('Push запланирован'); setScheduleAt(''); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: (e: any) => toast.error(e?.response?.data?.error || 'Ошибка планирования'),
  })
  const cancelSchedule = useMutation({
    mutationFn: (id: number) => api.delete(`/sync/schedule/${id}`),
    onSuccess: () => { toast.success('Отменено'); qc.invalidateQueries({ queryKey: ['schedules'] }) },
    onError: () => toast.error('Не удалось отменить'),
  })

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Push в магазин</Typography>
      {isLoading && <LinearProgress />}

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: '4px' }}>Отправка товаров в CS-Cart</Typography>
          <Typography sx={{ fontSize: 12, color: 'text.secondary', mb: 2 }}>
            Отправляет новые (ещё не отправленные) товары из выбранной категории на магазин.
            Статус товаров: <strong>{settings.default_product_status || 'A'}</strong>
          </Typography>
          <Stack spacing={2} sx={{ maxWidth: 560 }}>
            <FormControl size="small" fullWidth>
              <Select value={selectedCat} displayEmpty onChange={e => setSelectedCat(e.target.value as string)}>
                <MenuItem value=""><em>— выбери категорию —</em></MenuItem>
                {mappings.map(m => (
                  <MenuItem key={m.OTCategoryID} value={m.OTCategoryID}>
                    {m.OTCategoryID} → {m.CSCategoryName}{m.Notes && ` (${m.Notes})`}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <Stack direction="row" spacing={2} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 2 }}>
              <Button variant="contained" color="success" startIcon={<CloudUpload />}
                disabled={!selectedCat || pushMutation.isPending}
                onClick={() => pushMutation.mutate(selectedCat)}>
                Запустить Push сейчас
              </Button>
              <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', flexWrap: 'wrap', pl: { sm: 2 }, borderLeft: { sm: '1px solid' }, borderColor: { sm: 'divider' } }}>
                <TextField
                  type="datetime-local" size="small" label="Время запуска (ночь)"
                  value={scheduleAt} onChange={e => setScheduleAt(e.target.value)}
                  slotProps={{ inputLabel: { shrink: true } }} sx={{ minWidth: 230 }}
                />
                <Button variant="outlined" startIcon={<Schedule />}
                  disabled={!selectedCat || !scheduleAt || schedulePush.isPending}
                  onClick={() => schedulePush.mutate()}>
                  Запланировать Push
                </Button>
              </Box>
            </Stack>
            <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
              Push долгий (скачивание фото, создание карточек) — удобно ставить на ночь.
            </Typography>
          </Stack>
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
                    <TableCell>Категория</TableCell>
                    <TableCell>Время запуска</TableCell>
                    <TableCell>Статус</TableCell>
                    <TableCell align="right"></TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {pushSchedules.map((s: any) => (
                    <TableRow key={s.id} hover>
                      <TableCell>{s.label || s.category_id}</TableCell>
                      <TableCell>{ts(s.scheduled_at)}</TableCell>
                      <TableCell><Chip size="small" label={schedLabel(s.status)} color={schedColor(s.status)} /></TableCell>
                      <TableCell align="right">
                        {s.status === 'pending' && (
                          <Button size="small" color="error" onClick={() => cancelSchedule.mutate(s.id)} disabled={cancelSchedule.isPending}>
                            Отменить
                          </Button>
                        )}
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
