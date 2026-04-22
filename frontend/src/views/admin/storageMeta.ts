import type { StorageProvider, StorageProviderMeta } from '../../api'
import logoAlibabaCloud from '../../assets/storage-vendors/alibabacloud.svg'
import logoAmazonS3 from '../../assets/storage-vendors/amazons3.svg'
import logoCloudflare from '../../assets/storage-vendors/cloudflare.svg'
import logoMinio from '../../assets/storage-vendors/minio.svg'
import logoQiniu from '../../assets/storage-vendors/qiniu.png'
import logoTencentCloud from '../../assets/storage-vendors/tencentcloud.ico'

export interface VendorPreset {
  key: string
  provider: StorageProvider
  label: string
  logoSrc: string
  description: string
  defaultName: string
  defaultConfig?: Record<string, string | boolean>
}

export interface VendorPresetOption extends VendorPreset {
  supported: boolean
}

type ProviderMetaMap = Partial<Record<StorageProvider, StorageProviderMeta>>

export const vendorLogoMap: Partial<Record<StorageProvider, string>> = {
  kodo: logoQiniu,
  's3-compatible': logoAmazonS3,
  'tencent-cos': logoTencentCloud,
  'aws-s3': logoAmazonS3,
  'cloudflare-r2': logoCloudflare,
  minio: logoMinio,
}

export const vendorPresetCatalog: VendorPreset[] = [
  {
    key: 'kodo',
    provider: 'kodo',
    label: 'Kodo',
    logoSrc: vendorLogoMap.kodo as string,
    description: '七牛云对象存储，适合当前已接入的上传链路。',
    defaultName: 'Kodo 存储',
  },
  {
    key: 'aliyun-oss',
    provider: 's3-compatible',
    label: '阿里云 OSS',
    logoSrc: logoAlibabaCloud,
    description: '适合阿里云对象存储 OSS，默认走 S3 兼容接入方式。',
    defaultName: '阿里云 OSS 存储',
    defaultConfig: {
      endpoint: 'oss-cn-shanghai.aliyuncs.com',
      region_id: 'cn-shanghai',
      force_path_style: false,
    },
  },
  {
    key: 'tencent-cos',
    provider: 's3-compatible',
    label: '腾讯云 COS',
    logoSrc: vendorLogoMap['tencent-cos'] as string,
    description: '适合腾讯云体系项目，常用于图片与静态资源托管。',
    defaultName: '腾讯云 COS 存储',
    defaultConfig: {
      endpoint: 'cos.ap-shanghai.myqcloud.com',
      region_id: 'ap-shanghai',
      force_path_style: false,
    },
  },
  {
    key: 'aws-s3',
    provider: 's3-compatible',
    label: 'Amazon S3',
    logoSrc: vendorLogoMap['aws-s3'] as string,
    description: '标准 S3 协议对象存储，适合作为通用默认模板。',
    defaultName: 'Amazon S3 存储',
    defaultConfig: {
      endpoint: 's3.us-east-1.amazonaws.com',
      region_id: 'us-east-1',
      force_path_style: false,
    },
  },
  {
    key: 'cloudflare-r2',
    provider: 's3-compatible',
    label: 'Cloudflare R2',
    logoSrc: vendorLogoMap['cloudflare-r2'] as string,
    description: '适合全球分发与低出口成本的对象存储场景。',
    defaultName: 'Cloudflare R2 存储',
    defaultConfig: {
      endpoint: '<account-id>.r2.cloudflarestorage.com',
      region_id: 'auto',
      force_path_style: false,
    },
  },
  {
    key: 'minio',
    provider: 's3-compatible',
    label: 'MinIO',
    logoSrc: vendorLogoMap.minio as string,
    description: '适合私有部署和本地自建的 S3 兼容存储方案。',
    defaultName: 'MinIO 存储',
    defaultConfig: {
      endpoint: '127.0.0.1:9000',
      force_path_style: true,
    },
  },
]

export function buildVendorPresets(
  _providerMap: ProviderMetaMap,
  creatableProviders: StorageProviderMeta[]
): VendorPresetOption[] {
  return vendorPresetCatalog.map((vendor) => {
    const supported = creatableProviders.some((provider) => provider.provider === vendor.provider)
    return {
      ...vendor,
      supported,
    }
  })
}
