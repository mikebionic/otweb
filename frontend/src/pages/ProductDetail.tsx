import { useState } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Button, Chip,
  Grid, Stack, Divider, Switch, FormControlLabel, TextField,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  IconButton, Tooltip, Tab, Tabs,
} from '@mui/material'
import {
  ArrowBack, CloudUpload, Translate, CheckCircle, Edit, Save, Cancel,
  LocationOn, TrendingUp, Inventory2,
} from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { Product } from '../types'
import { imgProxy } from '../utils/imgProxy'

function tsColor(s: string): any {
  return s === 'done' ? 'success' : s === 'pending' ? 'warning' : s === 'error' ? 'error' : 'default'
}
function tsLabel(s: string) {
  return s === 'done' ? 'Переведён' : s === 'pending' ? 'Ожидает' : s === 'error' ? 'Ошибка' : 'Нет перевода'
}

export default function ProductDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const backTo = searchParams.get('back') || '/products'
  const qc = useQueryClient()

  const [tab, setTab] = useState(0)
  const [activeImg, setActiveImg] = useState(0)
  const [editMode, setEditMode] = useState(false)
  const [editFields, setEditFields] = useState({ title_ru: '', title_tk: '', desc_ru: '' })

  const { data, isLoading } = useQuery({
    queryKey: ['product', id],
    queryFn: () => api.get(`/products/${id}`).then(r => r.data.data),
    enabled: !!id,
  })

  const pushMut = useMutation({
    mutationFn: () => api.post(`/products/${id}/push`),
    onSuccess: () => { toast.success('Товар отправлен в CS-Cart'); qc.invalidateQueries({ queryKey: ['product', id] }) },
    onError: () => toast.error('Ошибка при пуше'),
  })

  const translateMut = useMutation({
    mutationFn: () => api.post(`/products/${id}/translate`, { action: 'auto' }),
    onSuccess: () => { toast.success('Перевод запущен'); qc.invalidateQueries({ queryKey: ['product', id] }) },
  })

  const saveMut = useMutation({
    mutationFn: () => api.post(`/products/${id}/translate`, {
      action: 'manual',
      title_ru: editFields.title_ru,
      title_tk: editFields.title_tk,
      desc_ru: editFields.desc_ru,
    }),
    onSuccess: () => {
      toast.success('Сохранено')
      setEditMode(false)
      qc.invalidateQueries({ queryKey: ['product', id] })
    },
    onError: () => toast.error('Ошибка'),
  })

  const toggleMut = useMutation({
    mutationFn: () => api.post(`/products/${id}/toggle`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['product', id] }),
  })

  const translateAttrsMut = useMutation({
    mutationFn: () => api.post(`/products/${id}/translate-attrs`),
    onSuccess: (r) => {
      const count = r.data.data?.translating ?? 0
      if (count === 0) {
        toast.success('Все характеристики уже переведены')
      } else {
        toast.success(`Перевод запущен (${count} пар)`)
        // Poll until untranslated_attrs === 0 or timeout
        const interval = setInterval(async () => {
          await qc.invalidateQueries({ queryKey: ['product', id] })
          const cached = qc.getQueryData<{ untranslated_attrs?: number }>(['product', id])
          if ((cached as any)?.untranslated_attrs === 0) clearInterval(interval)
        }, 3000)
        setTimeout(() => clearInterval(interval), 120000)
      }
    },
    onError: () => toast.error('Ошибка перевода'),
  })

  const p: Product | undefined = data?.product
  const images: { url: string }[] = data?.images ?? []
  const attrs: { name: string; value: string; name_ru: string; value_ru: string; is_configurator: boolean }[] = data?.attrs ?? []
  const skus: { sku_id: string; qty: number; price_cny: number }[] = data?.skus ?? []
  const untranslatedAttrs: number = data?.untranslated_attrs ?? 0

  const configurators = attrs.filter(a => a.is_configurator)
  const regularAttrs = attrs.filter(a => !a.is_configurator)

  function startEdit() {
    setEditFields({
      title_ru: p?.TitleRu || '',
      title_tk: p?.TitleTk || '',
      desc_ru: p?.DescriptionRU || '',
    })
    setEditMode(true)
  }

  if (isLoading) return <LinearProgress />
  if (!p) return <Typography sx={{ p: 3, color: 'text.secondary' }}>Товар не найден</Typography>

  const mainImg = imgProxy(images[activeImg]?.url || p.MainImageURL)

  return (
    <Box>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 3 }}>
        <IconButton onClick={() => navigate(backTo)} size="small">
          <ArrowBack />
        </IconButton>
        <Box sx={{ flexGrow: 1 }}>
          <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
            #{p.ID} · {p.CategoryID} · OtapiID: {p.OtapiID}
          </Typography>
          <Typography sx={{ fontSize: 18, fontWeight: 700, lineHeight: 1.3 }}>
            {p.TitleRu || p.TitleOriginal}
          </Typography>
        </Box>
        <Stack direction="row" spacing={1} sx={{ flexShrink: 0 }}>
          <FormControlLabel
            control={<Switch checked={p.Enabled} size="small" color="success" onChange={() => toggleMut.mutate()} />}
            label={<Typography sx={{ fontSize: 12 }}>{p.Enabled ? 'Включён' : 'Выключен'}</Typography>}
          />
          {p.CsProductID ? (
            <Chip
              icon={<CheckCircle sx={{ fontSize: 14 }} />}
              label={`CS-Cart #${p.CsProductID}`}
              color="success"
              size="small"
            />
          ) : (
            <Button
              variant="contained"
              size="small"
              startIcon={<CloudUpload />}
              onClick={() => pushMut.mutate()}
              disabled={pushMut.isPending}
            >
              Push в CS-Cart
            </Button>
          )}
        </Stack>
      </Box>

      <Grid container spacing={2}>
        {/* Left: images */}
        <Grid size={{ xs: 12, md: 4 }}>
          <Card>
            <CardContent sx={{ p: '12px !important' }}>
              {/* Main image */}
              <Box
                sx={{
                  width: '100%', aspectRatio: '1', borderRadius: 1.5, overflow: 'hidden',
                  background: '#f8f9fa', display: 'flex', alignItems: 'center', justifyContent: 'center', mb: 1,
                }}
              >
                {mainImg ? (
                  <img
                    src={mainImg}
                    alt=""
                    style={{ width: '100%', height: '100%', objectFit: 'contain' }}
                    onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
                  />
                ) : (
                  <Inventory2 sx={{ fontSize: 64, color: '#dee2e6' }} />
                )}
              </Box>
              {/* Thumbnails */}
              {images.length > 1 && (
                <Box sx={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                  {images.map((img, i) => (
                    <Box
                      key={i}
                      onClick={() => setActiveImg(i)}
                      sx={{
                        width: 52, height: 52, borderRadius: 1, overflow: 'hidden', cursor: 'pointer',
                        border: i === activeImg ? '2px solid' : '2px solid transparent',
                        borderColor: i === activeImg ? 'primary.main' : 'transparent',
                        background: '#f8f9fa', flexShrink: 0,
                      }}
                    >
                      <img src={imgProxy(img.url)} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                    </Box>
                  ))}
                </Box>
              )}
              {images.length === 0 && p.MainImageURL && (
                <Typography sx={{ fontSize: 11, color: 'text.secondary', textAlign: 'center' }}>
                  Основное фото
                </Typography>
              )}
            </CardContent>
          </Card>

          {/* Metrics */}
          <Card sx={{ mt: 2 }}>
            <CardContent sx={{ p: '12px !important' }}>
              <Typography sx={{ fontSize: 11, fontWeight: 600, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 1.5 }}>
                Метрики
              </Typography>
              <Stack spacing={1}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Цена (CNY)</Typography>
                  <Typography sx={{ fontSize: 14, fontWeight: 700 }}>¥{p.PriceCNY?.toFixed(2)}</Typography>
                </Box>
                <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Цена (TMT)</Typography>
                  <Typography sx={{ fontSize: 14, fontWeight: 700, color: 'primary.main' }}>{p.PriceTMT?.toFixed(2)} TMT</Typography>
                </Box>
                <Divider />
                {p.VolumeSales > 0 && (
                  <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Продаж</Typography>
                    <Chip icon={<TrendingUp sx={{ fontSize: 12 }} />} label={p.VolumeSales} size="small" color="success" variant="outlined" />
                  </Box>
                )}
                {(p.LocationStateRu || p.LocationCityRu) && (
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Локация</Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.3 }}>
                      <LocationOn sx={{ fontSize: 12, color: 'text.disabled' }} />
                      <Typography sx={{ fontSize: 12 }}>{p.LocationCityRu || p.LocationStateRu}</Typography>
                    </Box>
                  </Box>
                )}
                {skus.length > 0 && (
                  <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>Вариантов SKU</Typography>
                    <Typography sx={{ fontSize: 12, fontWeight: 600 }}>{skus.length}</Typography>
                  </Box>
                )}
              </Stack>
            </CardContent>
          </Card>
        </Grid>

        {/* Right: content */}
        <Grid size={{ xs: 12, md: 8 }}>
          <Card>
            <CardContent sx={{ p: '0 !important' }}>
              <Box sx={{ borderBottom: '1px solid #e9ecef', px: 2 }}>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ minHeight: 40, '& .MuiTab-root': { minHeight: 40, py: 0, fontSize: 13 } }}>
                  <Tab label="Перевод" />
                  <Tab label={`Характеристики (${attrs.length})`} />
                  <Tab label={`SKU (${skus.length})`} />
                </Tabs>
              </Box>

              {/* Tab: Translation */}
              {tab === 0 && (
                <Box sx={{ p: 2 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                    <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                      <Chip label={tsLabel(p.TranslateStatus)} color={tsColor(p.TranslateStatus)} size="small" />
                      {p.TranslateStatus !== 'done' && (
                        <Button size="small" variant="outlined" startIcon={<Translate />}
                          onClick={() => translateMut.mutate()} disabled={translateMut.isPending}>
                          Авто-перевод DeepSeek
                        </Button>
                      )}
                    </Box>
                    {!editMode ? (
                      <Button size="small" startIcon={<Edit />} onClick={startEdit}>Редактировать</Button>
                    ) : (
                      <Stack direction="row" spacing={1}>
                        <Button size="small" variant="contained" startIcon={<Save />}
                          onClick={() => saveMut.mutate()} disabled={saveMut.isPending}>
                          Сохранить
                        </Button>
                        <Button size="small" startIcon={<Cancel />} onClick={() => setEditMode(false)}>Отмена</Button>
                      </Stack>
                    )}
                  </Box>

                  <Stack spacing={2}>
                    <Box>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 0.5 }}>
                        Оригинал (ZH)
                      </Typography>
                      <Typography sx={{ fontSize: 13, color: '#495057', background: '#f8f9fa', p: 1, borderRadius: 1 }}>
                        {p.TitleOriginal}
                      </Typography>
                    </Box>

                    <Box>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 0.5 }}>
                        Название RU
                      </Typography>
                      {editMode ? (
                        <TextField fullWidth size="small" value={editFields.title_ru}
                          onChange={e => setEditFields(v => ({ ...v, title_ru: e.target.value }))} />
                      ) : (
                        <Typography sx={{ fontSize: 13, fontWeight: 500 }}>{p.TitleRu || <em style={{ color: '#adb5bd' }}>не переведено</em>}</Typography>
                      )}
                    </Box>

                    {(p.TitleTk || editMode) && (
                      <Box>
                        <Typography sx={{ fontSize: 11, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 0.5 }}>
                          Название TK
                        </Typography>
                        {editMode ? (
                          <TextField fullWidth size="small" value={editFields.title_tk}
                            onChange={e => setEditFields(v => ({ ...v, title_tk: e.target.value }))} />
                        ) : (
                          <Typography sx={{ fontSize: 13 }}>{p.TitleTk}</Typography>
                        )}
                      </Box>
                    )}

                    <Box>
                      <Typography sx={{ fontSize: 11, color: 'text.secondary', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 0.5 }}>
                        Описание RU
                      </Typography>
                      {editMode ? (
                        <TextField fullWidth multiline rows={5} size="small" value={editFields.desc_ru}
                          onChange={e => setEditFields(v => ({ ...v, desc_ru: e.target.value }))} />
                      ) : p.DescriptionRU ? (
                        <Typography sx={{ fontSize: 13, whiteSpace: 'pre-wrap', background: '#f8f9fa', p: 1, borderRadius: 1, maxHeight: 200, overflow: 'auto' }}>
                          {p.DescriptionRU}
                        </Typography>
                      ) : (
                        <Typography sx={{ fontSize: 12, color: '#adb5bd', fontStyle: 'italic' }}>Описания нет</Typography>
                      )}
                    </Box>
                  </Stack>
                </Box>
              )}

              {/* Tab: Attributes */}
              {tab === 1 && (
                <Box>
                  {/* Translate button */}
                  <Box sx={{ px: 2, pt: 1.5, pb: 1, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Typography sx={{ fontSize: 12, color: 'text.secondary' }}>
                      {untranslatedAttrs > 0
                        ? <span style={{ color: '#f59e0b' }}>⚠ {untranslatedAttrs} не переведено</span>
                        : <span style={{ color: '#10b981' }}>✓ Все переведены</span>}
                    </Typography>
                    {untranslatedAttrs > 0 && (
                      <Button size="small" variant="outlined" startIcon={<Translate />}
                        onClick={() => translateAttrsMut.mutate()} disabled={translateAttrsMut.isPending}>
                        DeepSeek перевод
                      </Button>
                    )}
                  </Box>

                  {configurators.length > 0 && (
                    <Box sx={{ px: 2, pb: 1 }}>
                      <Typography sx={{ fontSize: 11, fontWeight: 600, color: 'primary.main', textTransform: 'uppercase', letterSpacing: '0.05em', mb: 1 }}>
                        Конфигуратор (варианты)
                      </Typography>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                        {configurators.map((a, i) => (
                          <Chip
                            key={i}
                            label={`${a.name_ru || a.name}: ${a.value_ru || a.value}`}
                            title={`${a.name}: ${a.value}`}
                            size="small" color="primary" variant="outlined"
                          />
                        ))}
                      </Box>
                      <Divider sx={{ mt: 1.5 }} />
                    </Box>
                  )}
                  <TableContainer>
                    <Table size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell sx={{ width: '40%' }}>Характеристика</TableCell>
                          <TableCell>Значение</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {regularAttrs.map((a, i) => (
                          <TableRow key={i} hover>
                            <TableCell>
                              {a.name_ru ? (
                                <Typography sx={{ fontSize: 12, fontWeight: 500 }}>{a.name_ru}</Typography>
                              ) : (
                                <Typography sx={{ fontSize: 12, color: '#adb5bd', fontStyle: 'italic' }}>{a.name}</Typography>
                              )}
                            </TableCell>
                            <TableCell>
                              {a.value_ru ? (
                                <Typography sx={{ fontSize: 13 }}>{a.value_ru}</Typography>
                              ) : (
                                <Typography sx={{ fontSize: 12, color: '#adb5bd', fontStyle: 'italic' }}>{a.value}</Typography>
                              )}
                            </TableCell>
                          </TableRow>
                        ))}
                        {regularAttrs.length === 0 && (
                          <TableRow>
                            <TableCell colSpan={2} align="center" sx={{ py: 3, color: 'text.secondary', fontSize: 12 }}>
                              Нет характеристик
                            </TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                </Box>
              )}

              {/* Tab: SKUs */}
              {tab === 2 && (
                <TableContainer>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>SKU ID</TableCell>
                        <TableCell align="right">Цена CNY</TableCell>
                        <TableCell align="right">Кол-во</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {skus.map((s, i) => (
                        <TableRow key={i} hover>
                          <TableCell sx={{ fontSize: 12, fontFamily: 'monospace' }}>{s.sku_id}</TableCell>
                          <TableCell align="right" sx={{ fontWeight: 600 }}>¥{s.price_cny?.toFixed(2)}</TableCell>
                          <TableCell align="right" sx={{ color: s.qty > 0 ? 'success.main' : 'error.main', fontWeight: 600 }}>
                            {s.qty}
                          </TableCell>
                        </TableRow>
                      ))}
                      {skus.length === 0 && (
                        <TableRow>
                          <TableCell colSpan={3} align="center" sx={{ py: 3, color: 'text.secondary', fontSize: 12 }}>
                            Нет SKU данных
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>

          {/* Push again if already pushed */}
          {p.CsProductID > 0 && (
            <Box sx={{ mt: 1.5, display: 'flex', gap: 1, alignItems: 'center' }}>
              <Tooltip title="Обновить товар в CS-Cart">
                <Button variant="outlined" size="small" startIcon={<CloudUpload />}
                  onClick={() => pushMut.mutate()} disabled={pushMut.isPending}>
                  Переотправить в CS-Cart
                </Button>
              </Tooltip>
              <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                CS-Cart ID: #{p.CsProductID}
              </Typography>
            </Box>
          )}
        </Grid>
      </Grid>
    </Box>
  )
}
