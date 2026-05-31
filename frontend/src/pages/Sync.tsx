import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  FormControl, InputLabel, Select, MenuItem, TextField, Chip,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Switch, FormControlLabel, Collapse, IconButton, Divider, Alert,
  Accordion, AccordionSummary, AccordionDetails, Grid,
} from '@mui/material'
import { PlayArrow, ExpandMore, ExpandLess, Info } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { SyncJob, Category } from '../types'

function jobColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : 'default'
}
function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
function dur(start: number, end: number) {
  if (!start || !end) return '-'
  const s = end - start
  if (s < 60) return `${s}с`
  return `${Math.floor(s / 60)}м ${s % 60}с`
}

function JobLogPanel({ jobId }: { jobId: number }) {
  const [open, setOpen] = useState(false)
  const { data } = useQuery({
    queryKey: ['sync-job', jobId],
    queryFn: () => api.get(`/sync/jobs/${jobId}`).then(r => r.data.data),
    refetchInterval: open ? 2_000 : false,
    enabled: open,
  })
  return (
    <Box>
      <IconButton size="small" onClick={() => setOpen(v => !v)}>
        {open ? <ExpandLess sx={{ fontSize: 14 }} /> : <ExpandMore sx={{ fontSize: 14 }} />}
      </IconButton>
      <Collapse in={open}>
        <Box sx={{ background: '#1a1a2e', color: '#a8ff78', fontFamily: 'monospace', fontSize: 11, p: 1.5, mt: 0.5, borderRadius: 1, maxHeight: 200, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
          {data?.log || 'Лог пуст'}
        </Box>
      </Collapse>
    </Box>
  )
}

const ORDER_BY_OPTIONS = [
  { value: '',                label: 'По умолчанию (релевантность)' },
  { value: 'Volume:Desc',     label: 'По продажам (больше → меньше)' },
  { value: 'Price:Asc',       label: 'По цене (дешевле → дороже)' },
  { value: 'Price:Desc',      label: 'По цене (дороже → дешевле)' },
  { value: 'UpdatedTime:Desc',label: 'Сначала новые' },
]

const STUFF_STATUS_OPTIONS = [
  { value: '',    label: 'Все (новые + б/у)' },
  { value: 'New', label: 'Только новые товары' },
]

export default function Sync() {
  const [searchParams] = useSearchParams()
  const qc = useQueryClient()

  // Basic params
  const [categoryId, setCategoryId]     = useState(searchParams.get('category') ?? '')
  const [maxProducts, setMaxProducts]   = useState('500')
  const [pricesOnly, setPricesOnly]     = useState(false)

  // Price filters
  const [minPrice, setMinPrice]         = useState('')
  const [maxPrice, setMaxPrice]         = useState('')
  const [maxPriceLimit, setMaxPriceLimit] = useState('')

  // Advanced filters
  const [itemTitle, setItemTitle]             = useState('')
  const [vendorName, setVendorName]           = useState('')
  const [brandName, setBrandName]             = useState('')
  const [propertySearch, setPropertySearch]   = useState('')
  const [minVolume, setMinVolume]             = useState('')
  const [orderBy, setOrderBy]                 = useState('')
  const [stuffStatus, setStuffStatus]         = useState('')
  const [searchMethod, setSearchMethod]       = useState('')
  const [minVendorRating, setMinVendorRating] = useState('')
  const [maxVendorRating, setMaxVendorRating] = useState('')
  const [firstLotMin, setFirstLotMin]         = useState('')
  const [firstLotMax, setFirstLotMax]         = useState('')
  const [featureComplete, setFeatureComplete] = useState(false)
  const [featureDiscount, setFeatureDiscount] = useState(false)
  const [featureTmall, setFeatureTmall]       = useState(false)

  // History filters
  const [histStatus, setHistStatus]   = useState('')
  const [histJobType, setHistJobType] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['sync'],
    queryFn: () => api.get('/sync').then(r => r.data.data),
    refetchInterval: 5_000,
  })

  const runSync = useMutation({
    mutationFn: () => api.post('/sync/run', {
      category_id:      categoryId,
      max_products:     parseInt(maxProducts) || 500,
      min_price:        parseFloat(minPrice) || 0,
      max_price:        parseFloat(maxPrice) || 0,
      max_price_limit:  parseFloat(maxPriceLimit) || 0,
      prices_only:      pricesOnly,
      item_title:       itemTitle,
      vendor_name:       vendorName,
      brand_name:        brandName,
      property_search:   propertySearch,
      min_volume:        parseInt(minVolume) || 0,
      order_by:          orderBy,
      stuff_status:      stuffStatus,
      search_method:     searchMethod,
      min_vendor_rating: parseInt(minVendorRating) || 0,
      max_vendor_rating: parseInt(maxVendorRating) || 0,
      first_lot_min:     parseInt(firstLotMin) || 0,
      first_lot_max:     parseInt(firstLotMax) || 0,
      feature_complete:  featureComplete,
      feature_discount:  featureDiscount,
      feature_tmall:     featureTmall,
    }),
    onSuccess: r => { toast.success(`Задача #${r.data.data.job_id} запущена`); qc.invalidateQueries({ queryKey: ['sync'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const jobs: SyncJob[] = data?.jobs ?? []
  const cats: Category[] = data?.categories ?? []

  const filteredJobs = jobs.filter(j => {
    if (histStatus && j.Status !== histStatus) return false
    if (histJobType && j.JobType !== histJobType) return false
    return true
  })

  const runningJob = jobs.find(j => j.Status === 'running')

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Синхронизация</Typography>

      {runningJob && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Задача #{runningJob.ID} выполняется... Таблица обновляется каждые 5 сек.
        </Alert>
      )}

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>Запустить синхронизацию</Typography>
          <Stack spacing={2}>

            {/* Mode switcher */}
            <FormControlLabel
              control={<Switch checked={pricesOnly} onChange={e => setPricesOnly(e.target.checked)} />}
              label={
                <Box>
                  <Typography sx={{ fontSize: 13, fontWeight: 500 }}>
                    {pricesOnly ? 'Только цены' : 'Полная синхронизация товаров'}
                  </Typography>
                  <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                    {pricesOnly
                      ? 'Обновляет цены и наличие уже загруженных товаров. Быстро.'
                      : 'Загружает новые товары из 1688, обновляет описания, атрибуты, фото.'}
                  </Typography>
                </Box>
              }
            />

            <Divider />

            {/* Main params */}
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, md: 6 }}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Категория</InputLabel>
                  <Select value={categoryId} label="Категория" onChange={e => setCategoryId(e.target.value as string)}>
                    <MenuItem value="">Все включённые категории</MenuItem>
                    {cats.filter(c => c.Enabled).map(c => (
                      <MenuItem key={c.ID} value={c.ID}>
                        {c.Name}
                        <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>({c.ID})</Typography>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <TextField size="small" fullWidth label="Макс. товаров" type="number"
                  value={maxProducts} onChange={e => setMaxProducts(e.target.value)}
                  helperText="Сколько товаров загрузить максимум" />
              </Grid>
              <Grid size={{ xs: 6, md: 3 }}>
                <FormControl size="small" fullWidth>
                  <InputLabel>Сортировка API</InputLabel>
                  <Select value={orderBy} label="Сортировка API" onChange={e => setOrderBy(e.target.value)}>
                    {ORDER_BY_OPTIONS.map(o => <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>)}
                  </Select>
                </FormControl>
              </Grid>
            </Grid>

            {/* Price filters */}
            <Box>
              <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary', mb: 1 }}>Фильтр по цене (CNY)</Typography>
              <Alert severity="info" icon={<Info fontSize="small" />} sx={{ py: 0.5, mb: 1.5, '& .MuiAlert-message': { fontSize: 12 } }}>
                Мин/макс цена - фильтрует что запрашивать у 1688 API.
                Лимит аномалий - пропускает товары с ценой выше этого значения (защита от выбросов типа 50 000 CNY).
                0 = отключено.
              </Alert>
              <Stack direction="row" spacing={2} sx={{ flexWrap: 'wrap' }}>
                <TextField size="small" label="Мин. цена CNY" type="number"
                  value={minPrice} onChange={e => setMinPrice(e.target.value)}
                  sx={{ width: 150 }} helperText="0 = без минимума" />
                <TextField size="small" label="Макс. цена CNY" type="number"
                  value={maxPrice} onChange={e => setMaxPrice(e.target.value)}
                  sx={{ width: 150 }} helperText="0 = без максимума" />
                <TextField size="small" label="Лимит аномалий CNY" type="number"
                  value={maxPriceLimit} onChange={e => setMaxPriceLimit(e.target.value)}
                  sx={{ width: 175 }} helperText="Пропустить товары дороже N" placeholder="напр. 3000" />
              </Stack>
            </Box>

            {/* Advanced filters - collapsible */}
            {!pricesOnly && (
              <Accordion disableGutters elevation={0} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: '8px !important', '&:before': { display: 'none' } }}>
                <AccordionSummary expandIcon={<ExpandMore />}>
                  <Typography sx={{ fontSize: 13, fontWeight: 500 }}>Расширенные фильтры 1688</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Stack spacing={2}>
                    <Alert severity="info" icon={<Info fontSize="small" />} sx={{ py: 0.5, '& .MuiAlert-message': { fontSize: 12 } }}>
                      Все параметры передаются напрямую в API 1688. 0 = не фильтровать. Пустое поле = не применять.
                    </Alert>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Поиск товаров</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <TextField size="small" fullWidth label="Название товара" value={itemTitle}
                          onChange={e => setItemTitle(e.target.value)}
                          helperText="Ключевые слова на 1688 (китайский или английский)" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Бренд" value={brandName}
                          onChange={e => setBrandName(e.target.value)}
                          helperText="Фильтр по бренду товара" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Фильтр по свойству" value={propertySearch}
                          onChange={e => setPropertySearch(e.target.value)}
                          helperText="Формат pid:value (напр. 1627207:红色 для красного цвета)" />
                      </Grid>
                    </Grid>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Фильтр по продавцу</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Имя продавца" value={vendorName}
                          onChange={e => setVendorName(e.target.value)}
                          helperText="Фильтр по имени продавца" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Мин. рейтинг продавца" type="number" value={minVendorRating}
                          onChange={e => setMinVendorRating(e.target.value)}
                          helperText="Рейтинг 1-20+. Рекомендуется 8+" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 4 }}>
                        <TextField size="small" fullWidth label="Макс. рейтинг продавца" type="number" value={maxVendorRating}
                          onChange={e => setMaxVendorRating(e.target.value)}
                          helperText="Обычно не нужен (0 = без ограничения)" />
                      </Grid>
                    </Grid>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Объём и лот (только 1688)</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Мин. продаж" type="number" value={minVolume}
                          onChange={e => setMinVolume(e.target.value)}
                          helperText="Рекомендуется 50+" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Мин. первый лот" type="number" value={firstLotMin}
                          onChange={e => setFirstLotMin(e.target.value)}
                          helperText="Мин. кол-во в заказе. Обычно 1" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <TextField size="small" fullWidth label="Макс. первый лот" type="number" value={firstLotMax}
                          onChange={e => setFirstLotMax(e.target.value)}
                          helperText="Рекомендуется 10 (отсекает чисто оптовые)" />
                      </Grid>
                      <Grid size={{ xs: 6, md: 3 }}>
                        <FormControl size="small" fullWidth>
                          <InputLabel>Состояние товара</InputLabel>
                          <Select value={stuffStatus} label="Состояние товара" onChange={e => setStuffStatus(e.target.value)}>
                            {STUFF_STATUS_OPTIONS.map(o => <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>)}
                          </Select>
                        </FormControl>
                      </Grid>
                    </Grid>

                    <Typography sx={{ fontSize: 12, fontWeight: 600, color: 'text.secondary' }}>Метод поиска и особенности</Typography>
                    <Grid container spacing={2}>
                      <Grid size={{ xs: 12, md: 4 }}>
                        <FormControl size="small" fullWidth>
                          <InputLabel>Метод поиска</InputLabel>
                          <Select value={searchMethod} label="Метод поиска" onChange={e => setSearchMethod(e.target.value)}>
                            <MenuItem value="">По умолчанию (нативный 1688)</MenuItem>
                            <MenuItem value="Official">Official — только Tmall магазины</MenuItem>
                          </Select>
                        </FormControl>
                      </Grid>
                    </Grid>
                    <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap' }}>
                      <FormControlLabel
                        control={<Switch size="small" checked={featureComplete} onChange={e => setFeatureComplete(e.target.checked)} />}
                        label={
                          <Box>
                            <Typography sx={{ fontSize: 12 }}>Feature Complete</Typography>
                            <Typography sx={{ fontSize: 10, color: 'text.secondary' }}>Только полностью заполненные карточки</Typography>
                          </Box>
                        }
                      />
                      <FormControlLabel
                        control={<Switch size="small" checked={featureDiscount} onChange={e => setFeatureDiscount(e.target.checked)} />}
                        label={<Typography sx={{ fontSize: 12 }}>Только со скидкой</Typography>}
                      />
                      <FormControlLabel
                        control={<Switch size="small" checked={featureTmall} onChange={e => setFeatureTmall(e.target.checked)} />}
                        label={
                          <Box>
                            <Typography sx={{ fontSize: 12 }}>Feature Tmall</Typography>
                            <Typography sx={{ fontSize: 10, color: 'text.secondary' }}>Только Tmall (аналог SearchMethod=Official)</Typography>
                          </Box>
                        }
                      />
                    </Stack>
                  </Stack>
                </AccordionDetails>
              </Accordion>
            )}

            <Box>
              <Button variant="contained" size="large" startIcon={<PlayArrow />}
                onClick={() => runSync.mutate()} disabled={runSync.isPending || !!runningJob}>
                {runningJob ? 'Задача уже выполняется...' : 'Запустить'}
              </Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* History */}
      <Card>
        <CardContent>
          <Stack direction="row" spacing={2} sx={{ mb: 2, alignItems: 'center', flexWrap: 'wrap' }}>
            <Typography sx={{ fontWeight: 600 }}>История задач</Typography>
            <Box sx={{ flex: 1 }} />
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>Тип</InputLabel>
              <Select value={histJobType} label="Тип" onChange={e => setHistJobType(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="products">Товары</MenuItem>
                <MenuItem value="prices">Цены</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 130 }}>
              <InputLabel>Статус</InputLabel>
              <Select value={histStatus} label="Статус" onChange={e => setHistStatus(e.target.value)}>
                <MenuItem value="">Все</MenuItem>
                <MenuItem value="done">Завершено</MenuItem>
                <MenuItem value="running">Выполняется</MenuItem>
                <MenuItem value="error">Ошибка</MenuItem>
              </Select>
            </FormControl>
          </Stack>

          {isLoading && <LinearProgress />}

          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>#</TableCell>
                  <TableCell>Тип</TableCell>
                  <TableCell>Категория</TableCell>
                  <TableCell>Статус</TableCell>
                  <TableCell align="right">Загружено</TableCell>
                  <TableCell align="right">Пропущено</TableCell>
                  <TableCell align="right">Ошибок</TableCell>
                  <TableCell align="right">API запр.</TableCell>
                  <TableCell>Начало</TableCell>
                  <TableCell>Время</TableCell>
                  <TableCell>Лог</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredJobs.map(job => (
                  <TableRow key={job.ID} hover sx={job.Status === 'running' ? { bgcolor: 'action.hover' } : {}}>
                    <TableCell sx={{ color: 'text.secondary', fontSize: 12 }}>{job.ID}</TableCell>
                    <TableCell>
                      <Chip
                        label={job.JobType === 'prices' ? 'Цены' : 'Товары'}
                        size="small"
                        variant="outlined"
                        color={job.JobType === 'prices' ? 'warning' : 'primary'}
                      />
                    </TableCell>
                    <TableCell>
                      <Typography sx={{ fontSize: 12 }}>{job.CategoryID || 'все'}</Typography>
                    </TableCell>
                    <TableCell>
                      <Chip label={job.Status} color={jobColor(job.Status)} size="small" />
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, fontWeight: 600, color: (job.ItemsProcessed ?? 0) > 0 ? 'success.main' : 'text.disabled' }}>
                        {(job.ItemsProcessed ?? 0) > 0 ? `+${job.ItemsProcessed}` : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                        {(job.ItemsSkipped ?? 0) > 0 ? job.ItemsSkipped : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: (job.ErrorsCount ?? 0) > 0 ? 'error.main' : 'text.secondary' }}>
                        {(job.ErrorsCount ?? 0) > 0 ? job.ErrorsCount : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                        {(job.APIRequests ?? 0) > 0 ? job.APIRequests : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{ts(job.StartedAt)}</Typography>
                    </TableCell>
                    <TableCell>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{dur(job.StartedAt, job.FinishedAt)}</Typography>
                    </TableCell>
                    <TableCell>
                      <JobLogPanel jobId={job.ID} />
                    </TableCell>
                  </TableRow>
                ))}
                {filteredJobs.length === 0 && !isLoading && (
                  <TableRow>
                    <TableCell colSpan={11} align="center" sx={{ py: 4, color: 'text.secondary' }}>
                      Нет задач по выбранным фильтрам
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </Box>
  )
}
