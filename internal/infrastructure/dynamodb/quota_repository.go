package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/repository"
)

type dynamodbQuotaRepository struct {
	client    *dynamodb.Client
	tableName string
	fallback  *memoryQuotaRepository
}

type memoryQuotaRepository struct {
	mu             sync.RWMutex
	apiKeys        map[string]*entity.TenantContext      // apiKey -> TenantContext
	apiKeyRecords  map[string]*entity.APIKeyRecord       // apiKey -> APIKeyRecord
	tenantUsages   map[string]*entity.TenantMonthlyUsage // pk:sk -> TenantMonthlyUsage
	locks          map[string]time.Time                  // lockKey -> expiration
	serviceConfigs map[string]*entity.ServiceConfig      // serviceID -> ServiceConfig
	notifications  []*entity.Notification                // アプリ内通知リスト
	defaultQuota   int64
}

// NewQuotaRepository は DynamoDB クライアントを初期化し、QuotaRepository 実装を返す。
// 認証情報は環境変数（AWS_ACCESS_KEY_ID等）または IAM ロール（ECS TaskRole / IRSA）から
// AWS SDK 標準チェーンにより自動解決される。
func NewQuotaRepository(endpoint, region, tableName string, defaultQuota int64) repository.QuotaRepository {
	memRepo := newMemoryQuotaRepository(defaultQuota)

	if region == "" {
		region = "ap-northeast-1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		log.Printf("[WARN] Failed to load AWS config: %v. Falling back to in-memory store.", err)
		return memRepo
	}

	clientOpts := []func(*dynamodb.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	client := dynamodb.NewFromConfig(cfg, clientOpts...)

	log.Printf("[INFO] Initialized DynamoDB Quota Repository (Endpoint: %s, Region: %s, Table: %s)", endpoint, region, tableName)
	return &dynamodbQuotaRepository{
		client:    client,
		tableName: tableName,
		fallback:  memRepo,
	}
}

// GSIServiceUsage はサービス別月次集計用グローバルセカンダリインデックス名
const GSIServiceUsage = "GSI_ServiceUsage"

// --- DynamoDB Implementation ---

func (r *dynamodbQuotaRepository) FindTenantContextByAPIKey(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
	// 1. DynamoDB の APIKey レコードを検索
	keyRec, err := r.GetAPIKey(ctx, apiKey)
	if err == nil && keyRec != nil {
		if !keyRec.IsActive || keyRec.IsExpired() {
			return nil, nil // 失効済みまたは期限切れキー
		}
		return &entity.TenantContext{
			ServiceID:     keyRec.ServiceID,
			TenantID:      "default",
			UserID:        "anonymous",
			DataResidency: "global",
			APIKey:        keyRec.APIKey,
			AllowedModels: keyRec.AllowedModels,
			KeyCostLimit:  keyRec.CostLimit,
		}, nil
	}

	// 2. 命名規約またはフォールバック検索
	return r.fallback.FindTenantContextByAPIKey(ctx, apiKey)
}

func (r *dynamodbQuotaRepository) CreateAPIKey(ctx context.Context, record *entity.APIKeyRecord) error {
	record.PK = entity.BuildKeyPK(record.APIKey)
	record.SK = "METADATA"
	record.IsActive = true
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	// サービスマスター設定を取得。既に存在すればマスターのリミットに同期、なければ初期作成
	if existingCfg, _ := r.GetServiceConfig(ctx, record.ServiceID); existingCfg != nil {
		record.CostLimit = existingCfg.CostLimit
		record.BillingType = existingCfg.BillingType
	} else if record.CostLimit > 0 || record.BillingType != "" {
		_ = r.SetServiceConfig(ctx, &entity.ServiceConfig{
			ServiceID:   record.ServiceID,
			BillingType: record.BillingType,
			CostLimit:   record.CostLimit,
		})
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal API key record: %w", err)
	}

	// 1. キー検索用レコード (PK: KEY#<api_key>, SK: METADATA)
	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB PutItem for APIKey error, fallback to memory: %v", err)
		return r.fallback.CreateAPIKey(ctx, record)
	}

	// 2. サービス逆引き用レコード (PK: SERVICE#<service_id>, SK: KEY#<api_key>)
	serviceItem := make(map[string]types.AttributeValue)
	for k, v := range item {
		serviceItem[k] = v
	}
	serviceItem["pk"] = &types.AttributeValueMemberS{Value: entity.BuildServiceKeysPK(record.ServiceID)}
	serviceItem["sk"] = &types.AttributeValueMemberS{Value: entity.BuildKeySK(record.APIKey)}

	_, _ = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      serviceItem,
	})

	_ = r.fallback.CreateAPIKey(ctx, record)
	return nil
}

func (r *dynamodbQuotaRepository) GetAPIKey(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: entity.BuildKeyPK(apiKey)},
			"sk": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB GetItem for APIKey error, fallback to memory: %v", err)
		return r.fallback.GetAPIKey(ctx, apiKey)
	}
	if out.Item == nil {
		return r.fallback.GetAPIKey(ctx, apiKey)
	}

	var rec entity.APIKeyRecord
	if err := attributevalue.UnmarshalMap(out.Item, &rec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API key record: %w", err)
	}
	return &rec, nil
}

func (r *dynamodbQuotaRepository) ListAPIKeysByService(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
	var items []map[string]types.AttributeValue

	if serviceID != "" {
		out, err := r.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(r.tableName),
			KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :sk_prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":        &types.AttributeValueMemberS{Value: entity.BuildServiceKeysPK(serviceID)},
				":sk_prefix": &types.AttributeValueMemberS{Value: "KEY#"},
			},
		})
		if err != nil {
			log.Printf("[WARN] DynamoDB Query for service APIKeys error, fallback to memory: %v", err)
			return r.fallback.ListAPIKeysByService(ctx, serviceID)
		}
		items = out.Items
	} else {
		// 全サービスの API キーをスキャン取得
		out, err := r.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:        aws.String(r.tableName),
			FilterExpression: aws.String("begins_with(pk, :pk_prefix) AND begins_with(sk, :sk_prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk_prefix": &types.AttributeValueMemberS{Value: "SERVICE#"},
				":sk_prefix": &types.AttributeValueMemberS{Value: "KEY#"},
			},
		})
		if err != nil {
			log.Printf("[WARN] DynamoDB Scan for all APIKeys error, fallback to memory: %v", err)
			return r.fallback.ListAPIKeysByService(ctx, serviceID)
		}
		items = out.Items
	}

	var records []*entity.APIKeyRecord
	if err := attributevalue.UnmarshalListOfMaps(items, &records); err != nil {
		return nil, fmt.Errorf("failed to unmarshal API key records: %w", err)
	}

	// サービスマスター設定が存在すれば表示用リミットを常に同期
	if len(records) > 0 {
		configCache := make(map[string]*entity.ServiceConfig)
		for _, rec := range records {
			cfg, ok := configCache[rec.ServiceID]
			if !ok {
				cfg, _ = r.GetServiceConfig(ctx, rec.ServiceID)
				configCache[rec.ServiceID] = cfg
			}
			if cfg != nil {
				rec.CostLimit = cfg.CostLimit
				rec.BillingType = cfg.BillingType
			}
		}
	}

	return records, nil
}

func (r *dynamodbQuotaRepository) RevokeAPIKey(ctx context.Context, apiKey string) error {
	keyRec, err := r.GetAPIKey(ctx, apiKey)
	if err != nil || keyRec == nil {
		return r.fallback.RevokeAPIKey(ctx, apiKey)
	}

	nowISO := time.Now().UTC().Format(time.RFC3339)

	// 1. KEY#<api_key> レコードの無効化
	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: entity.BuildKeyPK(apiKey)},
			"sk": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String("SET is_active = :inactive, updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inactive": &types.AttributeValueMemberBOOL{Value: false},
			":now":      &types.AttributeValueMemberS{Value: nowISO},
		},
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB UpdateItem for APIKey revoke error, fallback to memory: %v", err)
	}

	// 2. SERVICE#<service_id> レコードの無効化
	_, _ = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: entity.BuildServiceKeysPK(keyRec.ServiceID)},
			"sk": &types.AttributeValueMemberS{Value: entity.BuildKeySK(apiKey)},
		},
		UpdateExpression: aws.String("SET is_active = :inactive, updated_at = :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inactive": &types.AttributeValueMemberBOOL{Value: false},
			":now":      &types.AttributeValueMemberS{Value: nowISO},
		},
	})

	return r.fallback.RevokeAPIKey(ctx, apiKey)
}

func (r *dynamodbQuotaRepository) GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
	pk := entity.BuildPK(serviceID, tenantID)
	sk := entity.BuildSK(month)

	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB GetItem error, fallback to memory: %v", err)
		return r.fallback.GetTenantUsage(ctx, serviceID, tenantID, month)
	}

	if out.Item == nil {
		return &entity.TenantMonthlyUsage{
			PK:          pk,
			SK:          sk,
			ServiceID:   serviceID,
			TenantID:    tenantID,
			Month:       month,
			TotalTokens: 0,
			TotalCost:   0,
			Models:      make(map[string]*entity.ModelUsage),
		}, nil
	}

	var usage entity.TenantMonthlyUsage
	if err := attributevalue.UnmarshalMap(out.Item, &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DynamoDB item: %w", err)
	}
	if usage.Models == nil {
		usage.Models = make(map[string]*entity.ModelUsage)
	}
	return &usage, nil
}

func (r *dynamodbQuotaRepository) IncrementTenantUsage(
	ctx context.Context,
	serviceID, tenantID, month string,
	model string,
	promptTokens, completionTokens int64,
	cost float64,
) error {
	pk := entity.BuildPK(serviceID, tenantID)
	sk := entity.BuildSK(month)
	totalTokens := promptTokens + completionTokens
	ttl := time.Now().AddDate(0, 0, 90).Unix()

	// 既存レコードを取得
	currentUsage, err := r.GetTenantUsage(ctx, serviceID, tenantID, month)
	if err != nil || currentUsage == nil {
		currentUsage = &entity.TenantMonthlyUsage{
			PK:        pk,
			SK:        sk,
			ServiceID: serviceID,
			TenantID:  tenantID,
			Month:     month,
			Models:    make(map[string]*entity.ModelUsage),
		}
	}

	currentUsage.TotalTokens += totalTokens
	currentUsage.TotalCost += cost
	currentUsage.UpdatedAt = time.Now().UTC()
	currentUsage.TTL = ttl

	if currentUsage.Models == nil {
		currentUsage.Models = make(map[string]*entity.ModelUsage)
	}
	cleanModel := strings.ReplaceAll(model, ".", "_")
	mUsage, ok := currentUsage.Models[cleanModel]
	if !ok {
		mUsage = &entity.ModelUsage{}
		currentUsage.Models[cleanModel] = mUsage
	}
	mUsage.PromptTokens += promptTokens
	mUsage.CompletionTokens += completionTokens
	mUsage.TotalTokens += totalTokens
	mUsage.Cost += cost

	item, err := attributevalue.MarshalMap(currentUsage)
	if err != nil {
		return err
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB PutItem failed, writing to fallback: %v", err)
		return r.fallback.IncrementTenantUsage(ctx, serviceID, tenantID, month, model, promptTokens, completionTokens, cost)
	}

	// フォールバックインメモリにも同期反映
	_ = r.fallback.IncrementTenantUsage(ctx, serviceID, tenantID, month, model, promptTokens, completionTokens, cost)
	return nil
}

func (r *dynamodbQuotaRepository) GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: entity.BuildServiceMetadataPK(serviceID)},
			"sk": &types.AttributeValueMemberS{Value: entity.BuildServiceMetadataSK()},
		},
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB GetServiceConfig error, fallback to memory: %v", err)
		return r.fallback.GetServiceConfig(ctx, serviceID)
	}
	if len(out.Item) == 0 {
		return r.fallback.GetServiceConfig(ctx, serviceID)
	}

	var cfg entity.ServiceConfig
	if err := attributevalue.UnmarshalMap(out.Item, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service config: %w", err)
	}
	return &cfg, nil
}

func (r *dynamodbQuotaRepository) SetServiceConfig(ctx context.Context, cfg *entity.ServiceConfig) error {
	cfg.PK = entity.BuildServiceMetadataPK(cfg.ServiceID)
	cfg.SK = entity.BuildServiceMetadataSK()
	cfg.UpdatedAt = time.Now().UTC()

	item, err := attributevalue.MarshalMap(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal service config: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB SetServiceConfig error, fallback to memory: %v", err)
		return r.fallback.SetServiceConfig(ctx, cfg)
	}

	_ = r.fallback.SetServiceConfig(ctx, cfg)
	return nil
}

func (r *dynamodbQuotaRepository) SetServiceLimit(
	ctx context.Context,
	serviceID string,
	costLimit float64,
	billingType string,
) error {
	// 1. 永続マスター設定 (SERVICE#<service_id> / METADATA) を保存
	svcConfig := &entity.ServiceConfig{
		ServiceID:   serviceID,
		BillingType: billingType,
		CostLimit:   costLimit,
	}
	_ = r.SetServiceConfig(ctx, svcConfig)

	// 2. 配下の API キーレコードの表示用メタデータも同期更新
	keys, err := r.ListAPIKeysByService(ctx, serviceID)
	if err == nil {
		for _, k := range keys {
			k.CostLimit = costLimit
			k.BillingType = billingType
			_ = r.CreateAPIKey(ctx, k)
		}
	}

	// 注: TenantMonthlyUsage は純粋な使用量集計レコードのため、不要な上限値カラムは書き込まない

	// インメモリフォールバックにも反映
	return r.fallback.SetServiceLimit(ctx, serviceID, costLimit, billingType)
}

func (r *dynamodbQuotaRepository) SetTenantLimit(
	ctx context.Context,
	serviceID, tenantID string,
	costLimit float64,
	billingType string,
) error {
	return r.SetServiceLimit(ctx, serviceID, costLimit, billingType)
}

func (r *dynamodbQuotaRepository) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	keyCond := "service_id = :svc AND #m = :month"
	exprAttrNames := map[string]string{
		"#m": "month",
	}
	exprAttrVals := map[string]types.AttributeValue{
		":svc":   &types.AttributeValueMemberS{Value: serviceID},
		":month": &types.AttributeValueMemberS{Value: month},
	}

	// GSI_ServiceUsage に対する高効率な Query を実行 (Scan による全件走査コストを完全撤廃)
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(r.tableName),
		IndexName:                 aws.String(GSIServiceUsage),
		KeyConditionExpression:    aws.String(keyCond),
		ExpressionAttributeNames:  exprAttrNames,
		ExpressionAttributeValues: exprAttrVals,
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB Query via GSI error (falling back to Scan/Memory): %v", err)
		// GSI 未作成・更新中のフェイルセーフとして Scan を試行
		scanOut, scanErr := r.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:                 aws.String(r.tableName),
			FilterExpression:          aws.String("service_id = :svc AND #m = :month"),
			ExpressionAttributeNames:  exprAttrNames,
			ExpressionAttributeValues: exprAttrVals,
		})
		if scanErr != nil {
			return r.fallback.GetServiceMonthlyUsage(ctx, serviceID, month)
		}
		out = &dynamodb.QueryOutput{
			Items: scanOut.Items,
		}
	}

	report := &entity.ServiceMonthlyReport{
		ServiceID: serviceID,
		Month:     month,
		Models:    make(map[string]*entity.ServiceReportModel),
		Tenants:   make(map[string]*entity.TenantReportItem),
	}

	for _, item := range out.Items {
		var usage entity.TenantMonthlyUsage
		if err := attributevalue.UnmarshalMap(item, &usage); err == nil {
			report.TotalTokens += usage.TotalTokens
			report.TotalCostUSD += usage.TotalCost

			// テナント別集計 (ショーバック・請求内訳用)
			tID := usage.TenantID
			if tID == "" {
				tID = "default"
			}
			if _, ok := report.Tenants[tID]; !ok {
				report.Tenants[tID] = &entity.TenantReportItem{
					TenantID: tID,
				}
			}
			report.Tenants[tID].TotalTokens += usage.TotalTokens
			report.Tenants[tID].TotalCostUSD += usage.TotalCost

			for mName, mVal := range usage.Models {
				origName := strings.ReplaceAll(mName, "_", ".")
				if _, ok := report.Models[origName]; !ok {
					report.Models[origName] = &entity.ServiceReportModel{}
				}
				report.Models[origName].Tokens += mVal.TotalTokens
				report.Models[origName].CostUSD += mVal.Cost
			}
		}
	}

	// サービスマスター設定から CostLimit と BillingType を反映
	if svcConfig, _ := r.GetServiceConfig(ctx, serviceID); svcConfig != nil {
		report.CostLimit = svcConfig.CostLimit
		report.BillingType = svcConfig.BillingType
	}

	return report, nil
}

func (r *dynamodbQuotaRepository) AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error) {
	pk := "LOCK#" + lockKey
	sk := "LOCK"
	now := time.Now()
	ttl := now.Add(time.Duration(ttlSeconds) * time.Second).Unix()

	item := map[string]types.AttributeValue{
		"pk":         &types.AttributeValueMemberS{Value: pk},
		"sk":         &types.AttributeValueMemberS{Value: sk},
		"lock_key":   &types.AttributeValueMemberS{Value: lockKey},
		"created_at": &types.AttributeValueMemberS{Value: now.UTC().Format(time.RFC3339)},
		"ttl":        &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)},
	}

	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			// 既に他コンテナによってロック獲得済み（正常なスキップ）
			return false, nil
		}
		log.Printf("[WARN] DynamoDB AcquireLock error: %v, trying fallback", err)
		return r.fallback.AcquireLock(ctx, lockKey, ttlSeconds)
	}

	// フォールバックインメモリにも登録
	_, _ = r.fallback.AcquireLock(ctx, lockKey, ttlSeconds)
	return true, nil
}

func (r *dynamodbQuotaRepository) GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error) {
	out, err := r.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:        aws.String(r.tableName),
		FilterExpression: aws.String("#m = :month AND begins_with(pk, :pk_prefix)"),
		ExpressionAttributeNames: map[string]string{
			"#m": "month",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":month":     &types.AttributeValueMemberS{Value: month},
			":pk_prefix": &types.AttributeValueMemberS{Value: "TENANT#"},
		},
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB GetAllTenantsUsageByMonth error: %v, fallback to memory", err)
		return r.fallback.GetAllTenantsUsageByMonth(ctx, month)
	}

	var results []*entity.TenantMonthlyUsage
	for _, item := range out.Items {
		var usage entity.TenantMonthlyUsage
		if err := attributevalue.UnmarshalMap(item, &usage); err == nil {
			results = append(results, &usage)
		}
	}
	return results, nil
}

func (r *dynamodbQuotaRepository) SaveNotification(ctx context.Context, ntf *entity.Notification) error {
	if ntf.CreatedAt.IsZero() {
		ntf.CreatedAt = time.Now().UTC()
	}
	if ntf.ID == "" {
		ntf.ID = fmt.Sprintf("ntf-%d", time.Now().UnixNano())
	}
	ntf.PK = entity.BuildNotificationPK()
	ntf.SK = entity.BuildNotificationSK(ntf.CreatedAt, ntf.ID)

	item, err := attributevalue.MarshalMap(ntf)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB PutItem for Notification failed, fallback to memory: %v", err)
		return r.fallback.SaveNotification(ctx, ntf)
	}
	return nil
}

func (r *dynamodbQuotaRepository) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: entity.BuildNotificationPK()},
		},
		ScanIndexForward: aws.Bool(false), // 降順 (最新が先頭)
		Limit:            aws.Int32(int32(limit)),
	})
	if err != nil {
		log.Printf("[WARN] DynamoDB Query for Notifications failed, fallback to memory: %v", err)
		return r.fallback.ListNotifications(ctx, limit)
	}

	var results []*entity.Notification
	for _, item := range out.Items {
		var ntf entity.Notification
		if err := attributevalue.UnmarshalMap(item, &ntf); err == nil {
			results = append(results, &ntf)
		}
	}
	return results, nil
}

func (r *dynamodbQuotaRepository) Ping(ctx context.Context) error {
	_, err := r.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(r.tableName),
	})
	if err != nil {
		return fmt.Errorf("dynamodb ping failed: %w", err)
	}
	return nil
}

// --- Memory Implementation ---

func newMemoryQuotaRepository(defaultQuota int64) *memoryQuotaRepository {
	// 本番用初期化: テスト用テナントやダミーデータは一切ハードコードせず空のマップで初期化
	return &memoryQuotaRepository{
		apiKeys:        make(map[string]*entity.TenantContext),
		apiKeyRecords:  make(map[string]*entity.APIKeyRecord),
		tenantUsages:   make(map[string]*entity.TenantMonthlyUsage),
		locks:          make(map[string]time.Time),
		serviceConfigs: make(map[string]*entity.ServiceConfig),
	}
}

func (m *memoryQuotaRepository) FindTenantContextByAPIKey(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. APIKeyRecord マップから検索
	rec, ok := m.apiKeyRecords[apiKey]
	if ok {
		if !rec.IsActive || rec.IsExpired() {
			return nil, nil // 失効済みまたは期限切れキー
		}
		return &entity.TenantContext{
			ServiceID:     rec.ServiceID,
			TenantID:      "default",
			UserID:        "anonymous",
			DataResidency: "global",
			APIKey:        rec.APIKey,
			AllowedModels: rec.AllowedModels,
			KeyCostLimit:  rec.CostLimit,
		}, nil
	}

	// 2. 既存の apiKeys マップから検索
	tc, ok := m.apiKeys[apiKey]
	if ok {
		return &entity.TenantContext{
			ServiceID:     tc.ServiceID,
			TenantID:      tc.TenantID,
			UserID:        tc.UserID,
			DataResidency: tc.DataResidency,
			APIKey:        tc.APIKey,
		}, nil
	}

	// 社内キー命名規約 "sk-internal-<service>" の場合は動的にサービス識別子を解決
	if strings.HasPrefix(apiKey, "sk-internal-") {
		svc := strings.TrimPrefix(apiKey, "sk-internal-")
		if svc != "" {
			return &entity.TenantContext{
				ServiceID:     svc,
				TenantID:      "default",
				UserID:        "anonymous",
				DataResidency: "global",
				APIKey:        apiKey,
			}, nil
		}
	}

	return nil, nil
}

func (m *memoryQuotaRepository) GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pk := entity.BuildPK(serviceID, tenantID)
	sk := entity.BuildSK(month)
	key := pk + ":" + sk

	usage, ok := m.tenantUsages[key]
	if !ok {
		usage := &entity.TenantMonthlyUsage{
			PK:          pk,
			SK:          sk,
			ServiceID:   serviceID,
			TenantID:    tenantID,
			Month:       month,
			TotalTokens: 0,
			TotalCost:   0,
			Models:      make(map[string]*entity.ModelUsage),
		}
		return usage, nil
	}

	// クローンして返却
	clone := *usage
	clone.Models = make(map[string]*entity.ModelUsage)
	for k, v := range usage.Models {
		clone.Models[k] = &entity.ModelUsage{
			PromptTokens:     v.PromptTokens,
			CompletionTokens: v.CompletionTokens,
			TotalTokens:      v.TotalTokens,
			Cost:             v.Cost,
		}
	}
	return &clone, nil
}

func (m *memoryQuotaRepository) IncrementTenantUsage(
	ctx context.Context,
	serviceID, tenantID, month string,
	model string,
	promptTokens, completionTokens int64,
	cost float64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pk := entity.BuildPK(serviceID, tenantID)
	sk := entity.BuildSK(month)
	key := pk + ":" + sk

	usage, ok := m.tenantUsages[key]
	if !ok {
		usage = &entity.TenantMonthlyUsage{
			PK:        pk,
			SK:        sk,
			ServiceID: serviceID,
			TenantID:  tenantID,
			Month:     month,
			Models:    make(map[string]*entity.ModelUsage),
		}
		m.tenantUsages[key] = usage
	}

	totalTokens := promptTokens + completionTokens
	usage.TotalTokens += totalTokens
	usage.TotalCost += cost
	usage.UpdatedAt = time.Now().UTC()

	if usage.Models == nil {
		usage.Models = make(map[string]*entity.ModelUsage)
	}
	cleanModel := strings.ReplaceAll(model, ".", "_")
	mUsage, ok := usage.Models[cleanModel]
	if !ok {
		mUsage = &entity.ModelUsage{}
		usage.Models[cleanModel] = mUsage
	}
	mUsage.PromptTokens += promptTokens
	mUsage.CompletionTokens += completionTokens
	mUsage.TotalTokens += totalTokens
	mUsage.Cost += cost

	return nil
}

func (m *memoryQuotaRepository) SetServiceLimit(
	ctx context.Context,
	serviceID string,
	costLimit float64,
	billingType string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	// 1. サービスマスター設定
	m.serviceConfigs[serviceID] = &entity.ServiceConfig{
		PK:          entity.BuildServiceMetadataPK(serviceID),
		SK:          entity.BuildServiceMetadataSK(),
		ServiceID:   serviceID,
		BillingType: billingType,
		CostLimit:   costLimit,
		UpdatedAt:   now,
	}

	// 2. 配下のキーメタデータ同期
	for _, rec := range m.apiKeyRecords {
		if rec.ServiceID == serviceID {
			rec.CostLimit = costLimit
			rec.BillingType = billingType
			rec.UpdatedAt = now
		}
	}

	return nil
}

func (m *memoryQuotaRepository) SetTenantLimit(
	ctx context.Context,
	serviceID, tenantID string,
	costLimit float64,
	billingType string,
) error {
	return m.SetServiceLimit(ctx, serviceID, costLimit, billingType)
}

func (m *memoryQuotaRepository) GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.serviceConfigs[serviceID]
	if !ok {
		return nil, nil
	}
	clone := *cfg
	return &clone, nil
}

func (m *memoryQuotaRepository) SetServiceConfig(ctx context.Context, cfg *entity.ServiceConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *cfg
	clone.PK = entity.BuildServiceMetadataPK(cfg.ServiceID)
	clone.SK = entity.BuildServiceMetadataSK()
	clone.UpdatedAt = time.Now().UTC()
	m.serviceConfigs[cfg.ServiceID] = &clone

	for _, rec := range m.apiKeyRecords {
		if rec.ServiceID == cfg.ServiceID {
			rec.CostLimit = cfg.CostLimit
			rec.BillingType = cfg.BillingType
			rec.UpdatedAt = clone.UpdatedAt
		}
	}
	return nil
}

func (m *memoryQuotaRepository) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := &entity.ServiceMonthlyReport{
		ServiceID:    serviceID,
		Month:        month,
		TotalTokens:  0,
		TotalCostUSD: 0,
		Models:       make(map[string]*entity.ServiceReportModel),
		Tenants:      make(map[string]*entity.TenantReportItem),
	}

	if cfg, ok := m.serviceConfigs[serviceID]; ok {
		report.CostLimit = cfg.CostLimit
		report.BillingType = cfg.BillingType
	}

	sk := entity.BuildSK(month)
	for _, usage := range m.tenantUsages {
		if usage.ServiceID == serviceID && usage.SK == sk {
			report.TotalTokens += usage.TotalTokens
			report.TotalCostUSD += usage.TotalCost

			// テナント別集計 (ショーバック・請求内訳用)
			tID := usage.TenantID
			if tID == "" {
				tID = "default"
			}
			if _, ok := report.Tenants[tID]; !ok {
				report.Tenants[tID] = &entity.TenantReportItem{
					TenantID: tID,
				}
			}
			report.Tenants[tID].TotalTokens += usage.TotalTokens
			report.Tenants[tID].TotalCostUSD += usage.TotalCost

			for mName, mVal := range usage.Models {
				origName := strings.ReplaceAll(mName, "_", ".")
				if _, ok := report.Models[origName]; !ok {
					report.Models[origName] = &entity.ServiceReportModel{}
				}
				report.Models[origName].Tokens += mVal.TotalTokens
				report.Models[origName].CostUSD += mVal.Cost
			}
		}
	}

	return report, nil
}

func (m *memoryQuotaRepository) CreateAPIKey(ctx context.Context, record *entity.APIKeyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	clone := *record
	clone.PK = entity.BuildKeyPK(record.APIKey)
	clone.SK = "METADATA"
	clone.IsActive = true
	now := time.Now().UTC()
	clone.CreatedAt = now
	clone.UpdatedAt = now

	m.apiKeyRecords[record.APIKey] = &clone
	return nil
}

func (m *memoryQuotaRepository) GetAPIKey(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rec, ok := m.apiKeyRecords[apiKey]
	if !ok {
		return nil, nil
	}
	clone := *rec
	return &clone, nil
}

func (m *memoryQuotaRepository) ListAPIKeysByService(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []*entity.APIKeyRecord
	for _, rec := range m.apiKeyRecords {
		if serviceID == "" || rec.ServiceID == serviceID {
			clone := *rec
			list = append(list, &clone)
		}
	}
	return list, nil
}

func (m *memoryQuotaRepository) RevokeAPIKey(ctx context.Context, apiKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, ok := m.apiKeyRecords[apiKey]
	if !ok {
		return nil
	}
	rec.IsActive = false
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *memoryQuotaRepository) AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if exp, exists := m.locks[lockKey]; exists && now.Before(exp) {
		return false, nil // 有効なロックが存在する（他コンテナが実行中）
	}

	m.locks[lockKey] = now.Add(time.Duration(ttlSeconds) * time.Second)
	return true, nil
}

func (m *memoryQuotaRepository) GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*entity.TenantMonthlyUsage
	for _, usage := range m.tenantUsages {
		if usage.Month == month {
			clone := *usage
			results = append(results, &clone)
		}
	}
	return results, nil
}

func (m *memoryQuotaRepository) SaveNotification(ctx context.Context, ntf *entity.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ntf.CreatedAt.IsZero() {
		ntf.CreatedAt = time.Now().UTC()
	}
	if ntf.ID == "" {
		ntf.ID = fmt.Sprintf("ntf-%d", time.Now().UnixNano())
	}
	ntf.PK = entity.BuildNotificationPK()
	ntf.SK = entity.BuildNotificationSK(ntf.CreatedAt, ntf.ID)

	clone := *ntf
	// 最新が先頭になるように prepend
	m.notifications = append([]*entity.Notification{&clone}, m.notifications...)
	return nil
}

func (m *memoryQuotaRepository) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var results []*entity.Notification
	for i, ntf := range m.notifications {
		if i >= limit {
			break
		}
		clone := *ntf
		results = append(results, &clone)
	}
	return results, nil
}

func (m *memoryQuotaRepository) Ping(ctx context.Context) error {
	return nil
}

