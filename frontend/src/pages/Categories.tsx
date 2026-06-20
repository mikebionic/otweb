import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, Typography, LinearProgress, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Chip, Switch, Tooltip,
  TextField, Select, MenuItem, FormControl, InputLabel, Stack,
  Button, IconButton, Tabs, Tab, Collapse,
} from '@mui/material'
import { Refresh, Translate, PlayArrow, ExpandMore, ExpandLess, Inventory2, DeleteOutlined } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { Category } from '../types'

function ProviderChip({ p }: { p: string }) {
  const m: Record<string, { label: string; color: any }> = {
    alibaba1688: { label: '1688', color: 'primary' },
    taobao: { label: 'Taobao', color: 'warning' },
    jd: { label: 'JD', color: 'error' },
    poizon: { label: 'Poizon', color: 'default' },
  }
  const v = m[p] ?? { label: p, color: 'default' }
  return <Chip label={v.label} color={v.color} size="small" />
}

function fmt(n: number) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return Math.round(n / 1_000) + 'K'
  return String(n)
}

function CategoryRow({ cat, depth = 0 }: { cat: Category; depth?: number }) {
  const [open, setOpen] = useState(false)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const hasKids = cat.Children && cat.Children.length > 0

  return (
    <>
      <TableRow hover sx={{ '& td': { borderBottom: hasKids && open ? 'none' : undefined }, opacity: cat.Enabled ? 1 : 0.45, bgcolor: cat.Enabled ? undefined : 'action.hover' }}>
        <TableCell sx={{ pl: 1.5 + depth * 2.5 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            {hasKids ? (
              <IconButton size="small" onClick={() => setOpen(v => !v)} sx={{ p: '2px' }}>
                {open ? <ExpandLess sx={{ fontSize: 16 }} /> : <ExpandMore sx={{ fontSize: 16 }} />}
              </IconButton>
            ) : (
              <Box sx={{ width: 26 }} />
            )}
            <Box>
              <Typography sx={{ fontSize: 13, fontWeight: depth === 0 ? 600 : 400 }}>{cat.Name}</Typography>
              <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{cat.NameEn || cat.NameZh || cat.ID}</Typography>
            </Box>
          </Box>
        </TableCell>
        <TableCell><ProviderChip p={cat.Provider} /></TableCell>
        <TableCell align="right">
          <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{fmt(cat.ItemCount)}</Typography>
        </TableCell>
        <TableCell align="right">
          {cat.LocalCount > 0
            ? <Typography sx={{ fontSize: 13, fontWeight: 600, color: 'success.main' }}>{cat.LocalCount}</Typography>
            : <Typography sx={{ color: 'text.disabled', fontSize: 12 }}>-</Typography>}
        </TableCell>
        <TableCell>
          {cat.CSCategoryName
            ? <Chip label={cat.CSCategoryName} color="success" size="small" sx={{ fontSize: 11 }} />
            : <Typography sx={{ fontSize: 11, color: 'text.disabled' }}>не замаплена</Typography>}
        </TableCell>
        <TableCell align="center">
          <Switch
            checked={cat.Enabled}
            size="small"
            color="success"
            onChange={() => api.post(`/categories/${cat.ID}/toggle`).then(() => qc.invalidateQueries({ queryKey: ['categories'] }))}
          />
        </TableCell>
        <TableCell align="right" sx={{ pr: 1.5 }}>
          <Stack direction="row" spacing={0.5} sx={{ justifyContent: 'flex-end' }}>
            {cat.LocalCount > 0 && (
              <Tooltip title="Смотреть товары">
                <IconButton size="small" sx={{ p: '3px' }} onClick={() => navigate(`/products?category=${cat.ID}`)}>
                  <Inventory2 sx={{ fontSize: 15 }} />
                </IconButton>
              </Tooltip>
            )}
            <Tooltip title="Синхронизировать сейчас">
              <IconButton size="small" color="primary" sx={{ p: '3px' }}
                onClick={() => navigate(`/sync?category=${cat.ID}`)}>
                <PlayArrow sx={{ fontSize: 15 }} />
              </IconButton>
            </Tooltip>
            <Tooltip title="Скрыть категорию (не показывать после синка)">
              <IconButton size="small" color="error" sx={{ p: '3px' }}
                onClick={() => { if (confirm(`Скрыть категорию ${cat.Name}?`)) api.post(`/categories/${cat.ID}/delete`).then(() => qc.invalidateQueries({ queryKey: ['categories'] })) }}>
                <DeleteOutlined sx={{ fontSize: 15 }} />
              </IconButton>
            </Tooltip>
          </Stack>
        </TableCell>
      </TableRow>
      {hasKids && (
        <TableRow sx={{ '& td': { p: 0, border: 0 } }}>
          <TableCell colSpan={7}>
            <Collapse in={open} unmountOnExit>
              <Box sx={{ borderLeft: '2px solid', borderColor: 'primary.light', ml: 4, my: '2px' }}>
                {cat.Children!.map(child => (
                  <CategoryRow key={child.ID} cat={child} depth={depth + 1} />
                ))}
              </Box>
            </Collapse>
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

const COLS = ['Категория', 'Провайдер', 'Товаров OT', 'Загружено', 'Маппинг CS-Cart', 'Авто-синк', '']

export default function Categories() {
  const [tab, setTab] = useState(0)
  const [provider, setProvider] = useState('')
  const [status, setStatus] = useState('')
  const [sort, setSort] = useState('items_desc')
  const [search, setSearch] = useState('')
  const qc = useQueryClient()
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['categories', provider, status, sort, search],
    queryFn: () => api.get('/categories', { params: { provider, status, sort, search } }).then(r => r.data.data),
  })

  const syncMeta = useMutation({
    mutationFn: () => api.post('/categories/sync-meta'),
    onSuccess: () => { toast.success('Метаданные обновлены'); qc.invalidateQueries({ queryKey: ['categories'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const translate = useMutation({
    mutationFn: () => api.post('/categories/translate'),
    onSuccess: () => toast.success('Перевод запущен'),
    onError: () => toast.error('Ошибка'),
  })

  const allFlat: Category[] = data?.categories ?? []
  // All categories, enabled first
  const flat: Category[] = [...allFlat]
    .sort((a, b) => (b.Enabled ? 1 : 0) - (a.Enabled ? 1 : 0))
  const tree: Category[] = data?.tree ?? []
  const total: number = data?.total ?? 0

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Box>
          <Typography sx={{ fontSize: 20, fontWeight: 700 }}>Категории</Typography>
          <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Всего: {total}</Typography>
        </Box>
        <Stack direction="row" spacing={1}>
          <Button variant="outlined" size="small" startIcon={<Translate />}
            onClick={() => translate.mutate()} disabled={translate.isPending}>
            AI-перевод
          </Button>
          <Button variant="contained" size="small" startIcon={<Refresh />}
            onClick={() => syncMeta.mutate()} disabled={syncMeta.isPending}>
            Обновить из API
          </Button>
        </Stack>
      </Box>

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2, minHeight: 38, '& .MuiTab-root': { minHeight: 38, py: 0 } }}>
        <Tab label="Список" />
        <Tab label="Дерево (родители → подкатегории)" />
      </Tabs>

      {tab === 0 && (
        <>
          <Stack direction="row" spacing={1} sx={{ mb: 2, flexWrap: 'wrap' }}>
            <FormControl size="small" sx={{ minWidth: 140 }}>
              <InputLabel>Провайдер</InputLabel>
              <Select value={provider} label="Провайдер" onChange={e => setProvider(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="alibaba1688">1688.com</MenuItem>
                <MenuItem value="taobao">Taobao</MenuItem>
                <MenuItem value="jd">JD</MenuItem>
                <MenuItem value="poizon">Poizon</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 140 }}>
              <InputLabel>Статус</InputLabel>
              <Select value={status} label="Статус" onChange={e => setStatus(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="enabled">Включённые</MenuItem>
                <MenuItem value="with_products">С товарами</MenuItem>
                <MenuItem value="mapped">Замапленные</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 160 }}>
              <InputLabel>Сортировка</InputLabel>
              <Select value={sort} label="Сортировка" onChange={e => setSort(e.target.value)}>
                <MenuItem value="items_desc">По кол-ву товаров</MenuItem>
                <MenuItem value="name">По названию</MenuItem>
                <MenuItem value="synced">По загруженным</MenuItem>
              </Select>
            </FormControl>
            <TextField size="small" placeholder="Поиск..." value={search}
              onChange={e => setSearch(e.target.value)} sx={{ width: 160 }} />
          </Stack>

          {isLoading && <LinearProgress sx={{ mb: 1 }} />}
          <Card>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    {COLS.map(c => <TableCell key={c}>{c}</TableCell>)}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {flat.map(cat => (
                    <TableRow key={cat.ID} hover sx={{ opacity: cat.Enabled ? 1 : 0.45, bgcolor: cat.Enabled ? undefined : 'action.hover' }}>
                      <TableCell sx={{ pl: 2 }}>
                        <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{cat.Name}</Typography>
                        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>{cat.NameEn || cat.NameZh || cat.ID}</Typography>
                      </TableCell>
                      <TableCell><ProviderChip p={cat.Provider} /></TableCell>
                      <TableCell align="right">
                        <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{fmt(cat.ItemCount)}</Typography>
                      </TableCell>
                      <TableCell align="right">
                        {cat.LocalCount > 0
                          ? <Typography sx={{ fontSize: 13, fontWeight: 600, color: 'success.main' }}>{cat.LocalCount}</Typography>
                          : <Typography sx={{ color: 'text.disabled', fontSize: 12 }}>-</Typography>}
                      </TableCell>
                      <TableCell>
                        {cat.CSCategoryName
                          ? <Chip label={cat.CSCategoryName} color="success" size="small" sx={{ fontSize: 11 }} />
                          : <Typography sx={{ fontSize: 11, color: 'text.disabled' }}>не замаплена</Typography>}
                      </TableCell>
                      <TableCell align="center">
                        <Switch checked={cat.Enabled} size="small" color="success"
                          onChange={() => api.post(`/categories/${cat.ID}/toggle`).then(() => qc.invalidateQueries({ queryKey: ['categories'] }))} />
                      </TableCell>
                      <TableCell align="right" sx={{ pr: 1.5 }}>
                        <Stack direction="row" spacing={0.5} sx={{ justifyContent: 'flex-end' }}>
                          {cat.LocalCount > 0 && (
                            <Tooltip title="Смотреть товары">
                              <IconButton size="small" sx={{ p: '3px' }} onClick={() => navigate(`/products?category=${cat.ID}`)}>
                                <Inventory2 sx={{ fontSize: 15 }} />
                              </IconButton>
                            </Tooltip>
                          )}
                          <Tooltip title="Синхронизировать сейчас">
                            <IconButton size="small" color="primary" sx={{ p: '3px' }} onClick={() => navigate(`/sync?category=${cat.ID}`)}>
                              <PlayArrow sx={{ fontSize: 15 }} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Скрыть категорию (не появится после синка)">
                            <IconButton size="small" color="error" sx={{ p: '3px' }}
                              onClick={() => { if (confirm(`Скрыть категорию ${cat.Name}?`)) api.post(`/categories/${cat.ID}/delete`).then(() => qc.invalidateQueries({ queryKey: ['categories'] })) }}>
                              <DeleteOutlined sx={{ fontSize: 15 }} />
                            </IconButton>
                          </Tooltip>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ))}
                  {!isLoading && flat.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={7} align="center" sx={{ py: 5, color: 'text.secondary' }}>
                        Нет категорий по выбранным фильтрам
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Card>
        </>
      )}

      {tab === 1 && (
        <>
          {isLoading && <LinearProgress sx={{ mb: 1 }} />}
          <Card>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    {COLS.map(c => <TableCell key={c}>{c}</TableCell>)}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {tree.map(node => <CategoryRow key={node.ID} cat={node} />)}
                  {!isLoading && tree.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={7} align="center" sx={{ py: 5, color: 'text.secondary' }}>
                        Нет данных
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </Card>
        </>
      )}
    </Box>
  )
}
