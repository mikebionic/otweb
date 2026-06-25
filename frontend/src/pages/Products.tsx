import { useState, useCallback } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, Typography, LinearProgress, Chip, Switch,
  TextField, Select, MenuItem, FormControl, InputLabel, Stack,
  Button, IconButton, Pagination, Avatar, Tooltip, ToggleButton, ToggleButtonGroup,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Collapse,
  Checkbox,
} from '@mui/material'
import {
  Translate, FilterList, CloudUpload, ViewModule, ViewList,
  TrendingUp, CheckCircle, FilterAlt, Delete, OpenInNew,
} from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { Product, Category } from '../types'
import { imgProxy } from '../utils/imgProxy'
import { displayTitle } from '../utils/lang'

// ── Card view ──────────────────────────────────────────────────────────
function ProductCard({ p, onPush }: { p: Product; onPush: (id: number) => void }) {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  function openDetail() {
    const back = '/products?' + searchParams.toString()
    navigate(`/products/${p.ID}?back=${encodeURIComponent(back)}`)
  }

  return (
    <Card
      sx={{
        cursor: 'pointer', transition: 'box-shadow 0.15s', height: '100%',
        display: 'flex', flexDirection: 'column',
        '&:hover': { boxShadow: '0 4px 16px rgba(0,0,0,0.12)' },
      }}
      onClick={openDetail}
    >
      {/* Image */}
      <Box sx={{ aspectRatio: '1', background: '#f8f9fa', overflow: 'hidden', position: 'relative', flexShrink: 0 }}>
        {p.MainImageURL ? (
          <img
            src={imgProxy(p.MainImageURL)}
            alt=""
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
            onError={e => { (e.target as HTMLImageElement).src = '' }}
          />
        ) : (
          <Box sx={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Typography sx={{ fontSize: 28, color: '#dee2e6' }}>📦</Typography>
          </Box>
        )}
        {/* Badges */}
        <Box sx={{ position: 'absolute', top: 6, left: 6, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
          {(p.QualityScore ?? 0) > 0 && (
            <Tooltip title={`Качество · Рейтинг: ${p.Rating ?? 0} · Положительных: ${p.GoodRates ?? 0}% · Заказов/30д: ${p.PayOrder30Day ?? 0}`}>
              <Chip
                label={`★ ${p.QualityScore}`}
                size="small"
                color={(p.QualityScore ?? 0) >= 70 ? 'success' : (p.QualityScore ?? 0) >= 45 ? 'warning' : 'error'}
                sx={{ fontSize: 10, height: 20, fontWeight: 700 }}
              />
            </Tooltip>
          )}
          {p.PushedToCsAt > 0 && (
            <Chip icon={<CheckCircle sx={{ fontSize: 10 }} />} label="CS" size="small" color="primary" sx={{ fontSize: 10, height: 20 }} />
          )}
        </Box>
        {/* Switch */}
        <Box sx={{ position: 'absolute', top: 4, right: 4 }} onClick={e => e.stopPropagation()}>
          <Switch
            checked={p.Enabled}
            size="small"
            color="success"
            onChange={() => api.post(`/products/${p.ID}/toggle`)}
            sx={{ transform: 'scale(0.8)' }}
          />
        </Box>
      </Box>

      {/* Content */}
      <Box sx={{ p: 1.5, flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
        <Typography sx={{ fontSize: 12, fontWeight: 500, lineHeight: 1.4, mb: 0.5, flexGrow: 1 }} style={{
          display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden',
        }}>
          {displayTitle(p.TitleRu, p.TitleOriginal)}
        </Typography>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 'auto' }}>
          <Typography sx={{ fontSize: 14, fontWeight: 700, color: 'primary.main' }}>
            {p.PriceTMT?.toFixed(0)} <span style={{ fontSize: 10, fontWeight: 400 }}>TMT</span>
          </Typography>
          <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>¥{p.PriceCNY?.toFixed(2)}</Typography>
        </Box>
        {p.VolumeSales > 0 && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.3, mt: 0.5 }}>
            <TrendingUp sx={{ fontSize: 11, color: 'success.main' }} />
            <Typography sx={{ fontSize: 10, color: 'success.main' }}>{p.VolumeSales} продаж</Typography>
          </Box>
        )}
      </Box>

      {/* Push button */}
      {p.PushedToCsAt === 0 && (
        <Box sx={{ px: 1.5, pb: 1.5 }} onClick={e => e.stopPropagation()}>
          <Button
            fullWidth size="small" variant="outlined" startIcon={<CloudUpload sx={{ fontSize: 14 }} />}
            onClick={() => onPush(p.ID)}
            sx={{ fontSize: 11, py: 0.3 }}
          >
            Push
          </Button>
        </Box>
      )}
    </Card>
  )
}

// ── Row view ──────────────────────────────────────────────────────────
function ProductRow({ p, onPush, selected, onSelect }: { p: Product; onPush: (id: number) => void; selected: boolean; onSelect: (id: number) => void }) {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const qc = useQueryClient()

  function openDetail() {
    const back = '/products?' + searchParams.toString()
    navigate(`/products/${p.ID}?back=${encodeURIComponent(back)}`)
  }

  return (
    <TableRow hover sx={{ cursor: 'pointer' }} onClick={openDetail} selected={selected}>
      <TableCell sx={{ width: 36, p: '0 4px' }} onClick={e => { e.stopPropagation(); onSelect(p.ID) }}>
        <Checkbox size="small" checked={selected} />
      </TableCell>
      <TableCell sx={{ width: 52, p: '6px 8px' }}>
        <Avatar
          src={imgProxy(p.MainImageURL)}
          variant="rounded"
          sx={{ width: 44, height: 44 }}
        />
      </TableCell>
      <TableCell sx={{ maxWidth: 260 }}>
        <Typography sx={{ fontSize: 12, fontWeight: 500, lineHeight: 1.4 }} style={{
          display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden',
        }}>
          {displayTitle(p.TitleRu, p.TitleOriginal)}
        </Typography>
        <Typography sx={{ fontSize: 10, color: 'text.disabled' }}>#{p.ID} · {p.CategoryID}</Typography>
      </TableCell>
      <TableCell align="right" sx={{ whiteSpace: 'nowrap' }}>
        <Typography sx={{ fontSize: 13, fontWeight: 700, color: 'primary.main' }}>{p.PriceTMT?.toFixed(0)} TMT</Typography>
        <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>¥{p.PriceCNY?.toFixed(2)}</Typography>
      </TableCell>
      <TableCell align="center">
        {p.VolumeSales > 0 && (
          <Chip icon={<TrendingUp sx={{ fontSize: 12 }} />} label={p.VolumeSales} size="small" color="success" variant="outlined" />
        )}
      </TableCell>
      <TableCell align="center">
        {(p.QualityScore ?? 0) > 0 && (
          <Tooltip title={`Рейтинг: ${p.Rating ?? 0} · Положительных: ${p.GoodRates ?? 0}% · Заказов/30д: ${p.PayOrder30Day ?? 0}`}>
            <Chip
              label={p.QualityScore}
              size="small"
              color={(p.QualityScore ?? 0) >= 70 ? 'success' : (p.QualityScore ?? 0) >= 45 ? 'warning' : 'error'}
              sx={{ fontWeight: 700, minWidth: 40 }}
            />
          </Tooltip>
        )}
        {(p.Rating ?? 0) > 0 && (
          <Typography sx={{ fontSize: 10, color: 'text.secondary', mt: 0.3 }}>★ {p.Rating} · {p.GoodRates}%</Typography>
        )}
      </TableCell>
      <TableCell align="center" onClick={e => e.stopPropagation()}>
        <Switch
          checked={p.Enabled} size="small" color="success"
          onChange={() => { api.post(`/products/${p.ID}/toggle`).then(() => qc.invalidateQueries({ queryKey: ['products'] })) }}
        />
      </TableCell>
      <TableCell align="center">
        {p.PushedToCsAt > 0
          ? <Chip icon={<CheckCircle sx={{ fontSize: 12 }} />} label="CS-Cart" color="primary" size="small" />
          : <Typography sx={{ color: 'text.disabled', fontSize: 11 }}>-</Typography>}
      </TableCell>
      <TableCell align="center" onClick={e => e.stopPropagation()}>
        {p.PlatformURL && (
          <Tooltip title="Открыть на сайте-доноре">
            <IconButton size="small" href={p.PlatformURL} target="_blank" rel="noopener">
              <OpenInNew sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        )}
        {p.PushedToCsAt === 0 && (
          <Tooltip title="Push в CS-Cart">
            <IconButton size="small" color="primary" onClick={() => onPush(p.ID)}>
              <CloudUpload sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        )}
      </TableCell>
    </TableRow>
  )
}

// ── Фильтр по свойству товара (pid + vid) ───────────────────────────────
function ProductPropertyFilter({ pid, vid, onChange }: {
  pid: string; vid: string; onChange: (pid: string, vid: string) => void
}) {
  const { data: props = [] } = useQuery<{ pid: string; label: string; count: number }[]>({
    queryKey: ['prod-properties'],
    queryFn: () => api.get('/sync/properties').then(r => r.data.data),
  })
  const { data: vals = [] } = useQuery<{ vid: string; value: string; label: string; count: number }[]>({
    queryKey: ['prod-prop-values', pid],
    queryFn: () => api.get('/sync/properties', { params: { pid } }).then(r => r.data.data),
    enabled: !!pid,
  })
  return (
    <>
      <FormControl size="small" sx={{ minWidth: 180 }}>
        <InputLabel>Свойство</InputLabel>
        <Select value={pid} label="Свойство" onChange={e => onChange(e.target.value as string, '')}>
          <MenuItem value="">Любое</MenuItem>
          {props.map(p => (
            <MenuItem key={p.pid} value={p.pid}>{p.label} <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>({p.count})</Typography></MenuItem>
          ))}
        </Select>
      </FormControl>
      {pid && (
        <FormControl size="small" sx={{ minWidth: 180 }}>
          <InputLabel>Значение</InputLabel>
          <Select value={vid} label="Значение" onChange={e => onChange(pid, e.target.value as string)}>
            <MenuItem value="">Любое значение</MenuItem>
            {vals.map(v => (
              <MenuItem key={v.vid} value={v.vid}>{v.label} <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>({v.count})</Typography></MenuItem>
            ))}
          </Select>
        </FormControl>
      )}
    </>
  )
}

// ── Main page ──────────────────────────────────────────────────────────
export default function Products() {
  const [searchParams] = useSearchParams()
  const [page, setPage] = useState(Number(searchParams.get('page')) || 1)
  const [category, setCategory] = useState(searchParams.get('category') ?? '')
  const [translateF, setTranslateF] = useState(searchParams.get('translate') ?? '')
  const [sort, setSort] = useState(searchParams.get('sort') ?? 'fetched')
  const [pushed, setPushed] = useState(searchParams.get('pushed') ?? '')
  const [gender, setGender] = useState(searchParams.get('gender') ?? '')
  const [age, setAge] = useState(searchParams.get('age') ?? '')
  const [search, setSearch] = useState(searchParams.get('search') ?? '')
  const [searchInput, setSearchInput] = useState(searchParams.get('search') ?? '')
  const [minQuality, setMinQuality] = useState(searchParams.get('min_quality') ?? '')
  const [propPid, setPropPid] = useState(searchParams.get('prop_pid') ?? '')
  const [propVid, setPropVid] = useState(searchParams.get('prop_vid') ?? '')
  const [fetchedAfter, setFetchedAfter] = useState(searchParams.get('fetched_after') ?? '')
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('list')
  const [showFilters, setShowFilters] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const qc = useQueryClient()

  const params = { page, per_page: viewMode === 'grid' ? 24 : 50, category, translate: translateF, sort, search, pushed: pushed === 'yes' ? 1 : '', unpushed: pushed === 'no' ? 1 : '', gender, age, prop_pid: propPid, prop_vid: propVid, fetched_after: fetchedAfter, min_quality: minQuality }

  const { data, isLoading } = useQuery({
    queryKey: ['products', params],
    queryFn: () => api.get('/products', { params }).then(r => r.data.data),
  })

  const translateAll = useMutation({
    mutationFn: () => api.post('/products/bulk-translate'),
    onSuccess: () => { toast.success('Перевод запущен'); qc.invalidateQueries({ queryKey: ['products'] }) },
  })

  const translateSelected = useMutation({
    mutationFn: (ids: number[]) => api.post('/products/bulk-translate', { product_ids: ids }),
    onSuccess: () => {
      toast.success(`Перевод ${selected.size} выбранных запущен`)
      setSelected(new Set())
      qc.invalidateQueries({ queryKey: ['products'] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const pushOne = useMutation({
    mutationFn: (id: number) => api.post(`/products/${id}/push`),
    onSuccess: () => { toast.success('Отправлен'); qc.invalidateQueries({ queryKey: ['products'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const bulkDelete = useMutation({
    mutationFn: (ids: number[]) => api.post('/products/bulk', { action: 'delete', product_ids: ids }),
    onSuccess: () => {
      toast.success(`Удалено ${selected.size} товаров`)
      setSelected(new Set())
      qc.invalidateQueries({ queryKey: ['products'] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const bulkPublish = useMutation({
    mutationFn: (ids: number[]) => api.post('/products/bulk', { action: 'publish', product_ids: ids }),
    onSuccess: () => {
      toast.success(`Публикация ${selected.size} товаров запущена`)
      setSelected(new Set())
      qc.invalidateQueries({ queryKey: ['products'] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const products: Product[] = data?.products ?? []
  const totalPages: number = data?.total_pages ?? 1
  const total: number = data?.total ?? 0
  const untranslated: number = data?.untranslated_count ?? 0
  const cats: Category[] = data?.categories ?? []

  const applySearch = useCallback(() => {
    setSearch(searchInput)
    setPage(1)
  }, [searchInput])

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const allSelected = products.length > 0 && products.every(p => selected.has(p.ID))
  const toggleSelectAll = () => {
    if (allSelected) {
      setSelected(prev => { const next = new Set(prev); products.forEach(p => next.delete(p.ID)); return next })
    } else {
      setSelected(prev => { const next = new Set(prev); products.forEach(p => next.add(p.ID)); return next })
    }
  }

  return (
    <Box>
      {/* Header */}
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
        <Box>
          <Typography sx={{ fontSize: 20, fontWeight: 700 }}>Товары</Typography>
          <Stack direction="row" spacing={1} sx={{ mt: 0.5 }}>
            <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Всего: {total}</Typography>
            {untranslated > 0 && (
              <Chip label={`${untranslated} без перевода`} size="small" color="warning" variant="outlined" />
            )}
            {fetchedAfter && (
              <Chip
                label={`Из синхронизации от ${new Date(Number(fetchedAfter) * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}`}
                size="small" color="info" variant="outlined"
                onDelete={() => setFetchedAfter('')}
              />
            )}
          </Stack>
        </Box>
        <Stack direction="row" spacing={1}>
          {selected.size > 0 && (
            <Button
              variant="contained" size="small" color="success" startIcon={<CheckCircle />}
              onClick={() => { if (confirm(`Опубликовать ${selected.size} товаров в CS-Cart? Товары будут включены и отправлены в магазин.`)) bulkPublish.mutate(Array.from(selected)) }}
              disabled={bulkPublish.isPending}
            >
              Опубликовать ({selected.size})
            </Button>
          )}
          {selected.size > 0 && (
            <Button
              variant="contained" size="small" color="primary" startIcon={<Translate />}
              onClick={() => translateSelected.mutate(Array.from(selected))}
              disabled={translateSelected.isPending}
            >
              Перевести выбранные ({selected.size})
            </Button>
          )}
          {selected.size > 0 && (
            <Button
              variant="outlined" size="small" color="error" startIcon={<Delete />}
              onClick={() => { if (confirm(`Удалить ${selected.size} товаров?`)) bulkDelete.mutate(Array.from(selected)) }}
              disabled={bulkDelete.isPending}
            >
              Удалить ({selected.size})
            </Button>
          )}
          <Button
            variant="outlined" size="small" startIcon={<FilterAlt />}
            onClick={() => setShowFilters(v => !v)}
            color={showFilters ? 'primary' : 'inherit'}
          >
            Фильтры
          </Button>
          <Button
            variant="outlined" size="small" startIcon={<Translate />}
            onClick={() => translateAll.mutate()} disabled={translateAll.isPending}
          >
            Перевести все
          </Button>
          <ToggleButtonGroup size="small" value={viewMode} exclusive onChange={(_, v) => v && setViewMode(v)}>
            <ToggleButton value="list"><ViewList /></ToggleButton>
            <ToggleButton value="grid"><ViewModule /></ToggleButton>
          </ToggleButtonGroup>
        </Stack>
      </Box>

      {/* Filter bar */}
      <Collapse in={showFilters}>
        <Card sx={{ mb: 2 }}>
          <Box sx={{ p: 1.5 }}>
            <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} sx={{ flexWrap: 'wrap' }}>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel>Категория</InputLabel>
                <Select value={category} label="Категория" onChange={e => { setCategory(e.target.value); setPage(1) }}>
                  <MenuItem value="">Все категории</MenuItem>
                  {cats.map(c => <MenuItem key={c.ID} value={c.ID}>{c.Path || c.Name}</MenuItem>)}
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 140 }}>
                <InputLabel>Пол</InputLabel>
                <Select value={gender} label="Пол" onChange={e => { setGender(e.target.value); setPage(1) }}>
                  <MenuItem value="">Любой</MenuItem>
                  <MenuItem value="male">Мальчики / Муж.</MenuItem>
                  <MenuItem value="female">Девочки / Жен.</MenuItem>
                  <MenuItem value="unisex">Унисекс</MenuItem>
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 150 }}>
                <InputLabel>Возраст</InputLabel>
                <Select value={age} label="Возраст" onChange={e => { setAge(e.target.value); setPage(1) }}>
                  <MenuItem value="">Любой</MenuItem>
                  <MenuItem value="baby">Baby (0–3)</MenuItem>
                  <MenuItem value="kids">Kids (3–8)</MenuItem>
                  <MenuItem value="children">Children (8+)</MenuItem>
                </Select>
              </FormControl>
              <ProductPropertyFilter
                pid={propPid}
                vid={propVid}
                onChange={(p, v) => { setPropPid(p); setPropVid(v); setPage(1) }}
              />
              <FormControl size="small" sx={{ minWidth: 150 }}>
                <InputLabel>Перевод</InputLabel>
                <Select value={translateF} label="Перевод" onChange={e => { setTranslateF(e.target.value); setPage(1) }}>
                  <MenuItem value="">Все</MenuItem>
                  <MenuItem value="done">Переведены</MenuItem>
                  <MenuItem value="pending">Ожидают</MenuItem>
                  <MenuItem value="none">Без перевода</MenuItem>
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 150 }}>
                <InputLabel>Push статус</InputLabel>
                <Select value={pushed} label="Push статус" onChange={e => { setPushed(e.target.value); setPage(1) }}>
                  <MenuItem value="">Все</MenuItem>
                  <MenuItem value="yes">Опубликованы в CS-Cart</MenuItem>
                  <MenuItem value="no">На ревью (не опубликованы)</MenuItem>
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 150 }}>
                <InputLabel>Мин. качество</InputLabel>
                <Select value={minQuality} label="Мин. качество" onChange={e => { setMinQuality(e.target.value); setPage(1) }}>
                  <MenuItem value="">Любое</MenuItem>
                  <MenuItem value="70">Высокое (70+)</MenuItem>
                  <MenuItem value="45">Среднее+ (45+)</MenuItem>
                </Select>
              </FormControl>
              <FormControl size="small" sx={{ minWidth: 180 }}>
                <InputLabel>Сортировка</InputLabel>
                <Select value={sort} label="Сортировка" onChange={e => { setSort(e.target.value); setPage(1) }}>
                  <MenuItem value="fetched">Новые сначала</MenuItem>
                  <MenuItem value="quality">По качеству</MenuItem>
                  <MenuItem value="price_asc">Цена ↑</MenuItem>
                  <MenuItem value="price_desc">Цена ↓</MenuItem>
                  <MenuItem value="sales">По продажам</MenuItem>
                  <MenuItem value="reviews">По отзывам</MenuItem>
                  <MenuItem value="fav">По избранным</MenuItem>
                </Select>
              </FormControl>
              <Box sx={{ display: 'flex', gap: 1 }}>
                <TextField
                  size="small" placeholder="Поиск по названию..." value={searchInput}
                  onChange={e => setSearchInput(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') applySearch() }}
                  sx={{ width: 220 }}
                />
                <IconButton size="small" onClick={applySearch}><FilterList /></IconButton>
                {(category || translateF || pushed || search || gender || age || propPid || fetchedAfter || minQuality) && (
                  <Button size="small" color="error" onClick={() => {
                    setCategory(''); setTranslateF(''); setPushed(''); setSearch(''); setSearchInput(''); setGender(''); setAge(''); setPropPid(''); setPropVid(''); setFetchedAfter(''); setMinQuality(''); setPage(1)
                  }}>
                    Сбросить
                  </Button>
                )}
              </Box>
            </Stack>
          </Box>
        </Card>
      </Collapse>

      {isLoading && <LinearProgress sx={{ mb: 1 }} />}

      {/* Grid view */}
      {viewMode === 'grid' && (
        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 2 }}>
          {products.map(p => (
            <ProductCard key={p.ID} p={p} onPush={id => pushOne.mutate(id)} />
          ))}
          {!isLoading && products.length === 0 && (
            <Box sx={{ gridColumn: '1/-1', py: 8, textAlign: 'center', color: 'text.secondary' }}>
              <Typography>Нет товаров по выбранным фильтрам</Typography>
            </Box>
          )}
        </Box>
      )}

      {/* List view */}
      {viewMode === 'list' && (
        <Card>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ width: 36, p: '0 4px' }}>
                    <Checkbox size="small" checked={allSelected} indeterminate={selected.size > 0 && !allSelected} onChange={toggleSelectAll} />
                  </TableCell>
                  <TableCell sx={{ width: 52 }}></TableCell>
                  <TableCell>Товар</TableCell>
                  <TableCell align="right">Цена</TableCell>
                  <TableCell align="center">Продажи</TableCell>
                  <TableCell align="center">Качество</TableCell>
                  <TableCell align="center">Вкл.</TableCell>
                  <TableCell align="center">CS-Cart</TableCell>
                  <TableCell align="center"></TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {products.map(p => (
                  <ProductRow key={p.ID} p={p} onPush={id => pushOne.mutate(id)} selected={selected.has(p.ID)} onSelect={toggleSelect} />
                ))}
                {!isLoading && products.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={10} align="center" sx={{ py: 6, color: 'text.secondary' }}>
                      Нет товаров по выбранным фильтрам
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </Card>
      )}

      {totalPages > 1 && (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
          <Pagination count={totalPages} page={page} onChange={(_, v) => setPage(v)} color="primary" />
        </Box>
      )}
    </Box>
  )
}
