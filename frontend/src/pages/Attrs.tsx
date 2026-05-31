import { useState, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  TextField, Select, MenuItem, FormControl, InputLabel,
  Checkbox, IconButton, Tooltip, Pagination, CircularProgress,
} from '@mui/material'
import {
  Translate, Edit, Check, Close, GTranslate,
} from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { AttrTranslation } from '../types'

function formatTs(ts: number) {
  if (!ts) return '-'
  return new Date(ts * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

interface AttrsData {
  total_all: number
  total_filtered: number
  translated: number
  pending: number
  page: number
  per_page: number
  items: AttrTranslation[]
}

interface EditState {
  pid: string
  vid: string
  field: 'name' | 'value'
  value: string
}

export default function Attrs() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [perPage] = useState(50)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [status, setStatus] = useState<'all' | 'translated' | 'pending'>('all')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [editing, setEditing] = useState<EditState | null>(null)

  const { data, isLoading } = useQuery<AttrsData>({
    queryKey: ['attrs', page, perPage, search, status],
    queryFn: () =>
      api.get('/attrs', { params: { page, per_page: perPage, search, status } })
        .then(r => r.data.data),
    refetchInterval: (query) => {
      const d = query.state.data as AttrsData | undefined
      return d && d.pending > 0 ? 5000 : false
    },
  })

  const translateAllMutation = useMutation({
    mutationFn: () => api.post('/attrs/translate'),
    onSuccess: () => {
      toast.success('Перевод запущен в фоне')
      qc.invalidateQueries({ queryKey: ['attrs'] })
    },
    onError: () => toast.error('Ошибка запуска перевода'),
  })

  const translateSelectedMutation = useMutation({
    mutationFn: (pairs: { pid: string; vid: string }[]) =>
      api.post('/attrs/translate-selected', { pairs }),
    onSuccess: () => {
      toast.success(`Переводим ${selected.size} пар`)
      setSelected(new Set())
      qc.invalidateQueries({ queryKey: ['attrs'] })
    },
    onError: () => toast.error('Ошибка перевода выбранных'),
  })

  const translateRowMutation = useMutation({
    mutationFn: (pair: { pid: string; vid: string }) =>
      api.post('/attrs/translate-selected', { pairs: [pair] }),
    onSuccess: () => {
      toast.success('Перевод запущен')
      qc.invalidateQueries({ queryKey: ['attrs'] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const saveMutation = useMutation({
    mutationFn: (body: { pid: string; vid: string; property_name_ru: string; value_ru: string }) =>
      api.post('/attrs/save', body),
    onSuccess: () => {
      toast.success('Сохранено')
      setEditing(null)
      qc.invalidateQueries({ queryKey: ['attrs'] })
    },
    onError: () => toast.error('Ошибка сохранения'),
  })

  const totalAll = data?.total_all ?? 0
  const totalFiltered = data?.total_filtered ?? 0
  const translated = data?.translated ?? 0
  const pending = data?.pending ?? 0
  const items: AttrTranslation[] = data?.items ?? []
  const pct = totalAll > 0 ? Math.round((translated / totalAll) * 100) : 0
  const totalPages = Math.max(1, Math.ceil(totalFiltered / perPage))

  const rowKey = (a: AttrTranslation) => `${a.pid}:${a.vid}`

  const allPageSelected = items.length > 0 && items.every(a => selected.has(rowKey(a)))
  const somePageSelected = items.some(a => selected.has(rowKey(a)))

  const toggleSelectAll = useCallback(() => {
    if (allPageSelected) {
      const next = new Set(selected)
      items.forEach(a => next.delete(rowKey(a)))
      setSelected(next)
    } else {
      const next = new Set(selected)
      items.forEach(a => next.add(rowKey(a)))
      setSelected(next)
    }
  }, [allPageSelected, items, selected])

  const toggleRow = useCallback((key: string) => {
    const next = new Set(selected)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setSelected(next)
  }, [selected])

  const handleSearch = () => {
    setSearch(searchInput)
    setPage(1)
  }

  const startEdit = (a: AttrTranslation, field: 'name' | 'value') => {
    setEditing({
      pid: a.pid,
      vid: a.vid,
      field,
      value: field === 'name' ? a.property_name_ru : a.value_ru,
    })
  }

  const commitEdit = (a: AttrTranslation) => {
    if (!editing) return
    const nameRu = editing.field === 'name' ? editing.value : a.property_name_ru
    const valueRu = editing.field === 'value' ? editing.value : a.value_ru
    saveMutation.mutate({ pid: a.pid, vid: a.vid, property_name_ru: nameRu, value_ru: valueRu })
  }

  const EditableCell = ({ a, field }: { a: AttrTranslation; field: 'name' | 'value' }) => {
    const isEditing = editing?.pid === a.pid && editing?.vid === a.vid && editing?.field === field
    const text = field === 'name' ? a.property_name_ru : a.value_ru
    if (isEditing) {
      return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <TextField
            size="small"
            value={editing.value}
            onChange={e => setEditing({ ...editing, value: e.target.value })}
            onKeyDown={e => { if (e.key === 'Enter') commitEdit(a); if (e.key === 'Escape') setEditing(null) }}
            autoFocus
            sx={{ minWidth: 140 }}
            slotProps={{ htmlInput: { style: { fontSize: 12 } } }}
          />
          <IconButton size="small" onClick={() => commitEdit(a)} disabled={saveMutation.isPending}>
            {saveMutation.isPending ? <CircularProgress size={14} /> : <Check fontSize="small" color="success" />}
          </IconButton>
          <IconButton size="small" onClick={() => setEditing(null)}>
            <Close fontSize="small" />
          </IconButton>
        </Box>
      )
    }
    return (
      <Box
        sx={{ display: 'flex', alignItems: 'center', gap: 0.5, cursor: 'pointer', '&:hover .edit-icon': { opacity: 1 } }}
        onClick={() => startEdit(a, field)}
      >
        <Typography sx={{ fontSize: 12, fontWeight: 500 }}>{text || <span style={{ color: '#bbb', fontStyle: 'italic' }}>—</span>}</Typography>
        <Edit className="edit-icon" sx={{ fontSize: 14, color: 'text.secondary', opacity: 0, transition: 'opacity 0.15s' }} />
      </Box>
    )
  }

  return (
    <Box>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography sx={{ fontSize: 18, fontWeight: 700 }}>Атрибуты товаров</Typography>
      </Box>

      {isLoading && <LinearProgress sx={{ mb: 1 }} />}

      {/* Stats */}
      <Stack direction="row" spacing={2} sx={{ mb: 2 }}>
        {[
          { label: 'Всего', value: totalAll, color: 'text.primary' },
          { label: 'Переведено', value: translated, color: 'success.main' },
          { label: 'Ожидают', value: pending, color: 'warning.main' },
        ].map(s => (
          <Card key={s.label} sx={{ flex: 1 }}>
            <CardContent sx={{ py: '12px !important', px: 2 }}>
              <Typography sx={{ fontSize: 11, textTransform: 'uppercase', color: 'text.secondary', letterSpacing: '0.05em' }}>
                {s.label}
              </Typography>
              <Typography sx={{ fontSize: 26, fontWeight: 700, color: s.color }}>{s.value}</Typography>
            </CardContent>
          </Card>
        ))}
      </Stack>

      {/* Progress bar */}
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: '10px !important', px: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: '4px' }}>
            <Typography sx={{ fontSize: 13 }}>Прогресс перевода</Typography>
            <Typography sx={{ fontSize: 13, fontWeight: 600 }}>{pct}%</Typography>
          </Box>
          <Box sx={{ height: 8, borderRadius: 4, background: '#e9ecef', overflow: 'hidden' }}>
            <Box sx={{ height: '100%', width: `${pct}%`, bgcolor: 'primary.main', transition: 'width 0.3s' }} />
          </Box>
        </CardContent>
      </Card>

      {/* Toolbar */}
      <Card sx={{ mb: 2 }}>
        <CardContent sx={{ py: '10px !important', px: 2 }}>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
            <TextField
              size="small"
              placeholder="Поиск на любом языке..."
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
              sx={{ width: 220 }}
            />
            <Button size="small" variant="outlined" onClick={handleSearch}>Найти</Button>

            <FormControl size="small" sx={{ minWidth: 150 }}>
              <InputLabel>Статус</InputLabel>
              <Select
                value={status}
                label="Статус"
                onChange={e => { setStatus(e.target.value as typeof status); setPage(1) }}
              >
                <MenuItem value="all">Все</MenuItem>
                <MenuItem value="translated">Переведённые</MenuItem>
                <MenuItem value="pending">Ожидают</MenuItem>
              </Select>
            </FormControl>

            <Box sx={{ flex: 1 }} />

            {selected.size > 0 && (
              <Button
                variant="outlined"
                size="small"
                startIcon={<GTranslate />}
                onClick={() => {
                  const pairs = Array.from(selected).map(k => {
                    const [pid, vid] = k.split(':')
                    return { pid, vid }
                  })
                  translateSelectedMutation.mutate(pairs)
                }}
                disabled={translateSelectedMutation.isPending}
              >
                Перевести выбранные ({selected.size})
              </Button>
            )}

            <Button
              variant="contained"
              size="small"
              startIcon={<Translate />}
              onClick={() => translateAllMutation.mutate()}
              disabled={translateAllMutation.isPending || pending === 0}
            >
              Перевести все{pending > 0 ? ` (${pending})` : ''}
            </Button>
          </Stack>
        </CardContent>
      </Card>

      {/* Table */}
      <Card>
        <CardContent sx={{ pb: '12px !important', px: 1 }}>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      indeterminate={somePageSelected && !allPageSelected}
                      checked={allPageSelected}
                      onChange={toggleSelectAll}
                    />
                  </TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Атрибут ZH</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Атрибут RU</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Значение ZH</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Значение RU</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Статус</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }}>Дата</TableCell>
                  <TableCell sx={{ fontSize: 12, fontWeight: 600 }} />
                </TableRow>
              </TableHead>
              <TableBody>
                {items.map(a => {
                  const key = rowKey(a)
                  const isTranslated = !!a.property_name_ru
                  return (
                    <TableRow key={key} hover selected={selected.has(key)}>
                      <TableCell padding="checkbox">
                        <Checkbox size="small" checked={selected.has(key)} onChange={() => toggleRow(key)} />
                      </TableCell>
                      <TableCell>
                        <Typography sx={{ fontSize: 12 }}>{a.property_name_zh}</Typography>
                        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{a.pid}</Typography>
                      </TableCell>
                      <TableCell>
                        <EditableCell a={a} field="name" />
                      </TableCell>
                      <TableCell>
                        <Typography sx={{ fontSize: 12 }}>{a.value_zh}</Typography>
                        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{a.vid}</Typography>
                      </TableCell>
                      <TableCell>
                        <EditableCell a={a} field="value" />
                      </TableCell>
                      <TableCell>
                        {isTranslated
                          ? <Typography sx={{ fontSize: 14, color: 'success.main' }}>✓</Typography>
                          : <Typography sx={{ fontSize: 14, color: 'warning.main' }}>⏳</Typography>
                        }
                      </TableCell>
                      <TableCell>
                        <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                          {formatTs(a.translated_at)}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Tooltip title="Перевести">
                          <IconButton
                            size="small"
                            onClick={() => translateRowMutation.mutate({ pid: a.pid, vid: a.vid })}
                          >
                            <GTranslate fontSize="small" sx={{ color: 'warning.main' }} />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  )
                })}
                {!isLoading && items.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={8} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                      Нет данных
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {totalPages > 1 && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
              <Pagination
                count={totalPages}
                page={page}
                onChange={(_, v) => { setPage(v); setSelected(new Set()) }}
                color="primary"
                size="small"
              />
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  )
}
