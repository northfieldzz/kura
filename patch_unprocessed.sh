cat << 'INNER_EOF' > /tmp/unprocessed_patch.txt
<<<<<<< SEARCH
		items := out.Responses[r.tableName]
		for _, item := range items {
			var cfg entity.ServiceConfig
			if err := attributevalue.UnmarshalMap(item, &cfg); err == nil {
				result[cfg.ServiceID] = &cfg
			}
		}
	}

	return result, nil
=======
		items := out.Responses[r.tableName]
		for _, item := range items {
			var cfg entity.ServiceConfig
			if err := attributevalue.UnmarshalMap(item, &cfg); err == nil {
				result[cfg.ServiceID] = &cfg
			}
		}

		// Handle unprocessed keys robustly by falling back to memory for those specific keys
		if len(out.UnprocessedKeys) > 0 {
			if keysAttr, ok := out.UnprocessedKeys[r.tableName]; ok {
				var unprocessedIDs []string
				for _, keyMap := range keysAttr.Keys {
					if pkAttr, pkOk := keyMap["pk"]; pkOk {
						if pkStr, pkIsStr := pkAttr.(*types.AttributeValueMemberS); pkIsStr {
							// pk format is "SERVICE#" + serviceID
							if strings.HasPrefix(pkStr.Value, "SERVICE#") {
								unprocessedIDs = append(unprocessedIDs, strings.TrimPrefix(pkStr.Value, "SERVICE#"))
							}
						}
					}
				}
				if len(unprocessedIDs) > 0 {
					log.Printf("[WARN] DynamoDB GetServiceConfigs had %d unprocessed keys, fallback to memory", len(unprocessedIDs))
					memConfigs, _ := r.fallback.GetServiceConfigs(ctx, unprocessedIDs)
					for k, v := range memConfigs {
						result[k] = v
					}
				}
			}
		}
	}

	return result, nil
>>>>>>> REPLACE
INNER_EOF
