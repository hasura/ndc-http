package contenttype

import (
	"bytes"
	"testing"

	"github.com/hasura/ndc-sdk-go/schema"
	"gotest.tools/v3/assert"
)

func TestJSONEncode(t *testing.T) {
	testCases := []struct {
		Name       string
		Input      string
		ResultType schema.Type
	}{
		{
			Name:       "PostCheckoutSessionsBodyObjectInput",
			Input:      `{}`,
			ResultType: schema.NewNamedType("PostCheckoutSessionsBodyObjectInput").Encode(),
		},
		{
			Name: "PostCheckoutSessionsBodyObjectInput",
			Input: `{
				"after_expiration": {
				  "recovery": {
					"allow_promotion_codes": true,
					"enabled": true
				  }
				},
				"allow_promotion_codes": true,
				"automatic_tax": {
				  "enabled": false,
				  "liability": {
					"account": "gW7D0WhP9C",
					"type": "self"
				  }
				},
				"billing_address_collection": "auto",
				"cancel_url": "qpmWppPyIv",
				"client_reference_id": "ZcJeCf6JAa",
				"consent_collection": {
				  "payment_method_reuse_agreement": {
					"position": "auto"
				  },
				  "promotions": "auto",
				  "terms_of_service": "required"
				},
				"currency": "oVljMB8lon",
				"custom_fields": [
				  {
					"dropdown": {
					  "options": [
						{
						  "label": "W3oysCi31d",
						  "value": "hXN8MppU0k"
						}
					  ]
					},
					"key": "5ZeyjIHLn8",
					"label": {
					  "custom": "uabTz3xzdn",
					  "type": "custom"
					},
					"numeric": {
					  "maximum_length": 678468035,
					  "minimum_length": 2134997439
					},
					"optional": false,
					"text": {
					  "maximum_length": 331815114,
					  "minimum_length": 1689246767
					},
					"type": "dropdown"
				  }
				],
				"custom_text": {
				  "after_submit": {
					"message": "b7ifuedi9S"
				  },
				  "shipping_address": {
					"message": "XeD5TkmC8k"
				  },
				  "submit": {
					"message": "vGcSz5eSlo"
				  },
				  "terms_of_service_acceptance": {
					"message": "zGLTTZItPl"
				  }
				},
				"customer": "mT4BKOSu9s",
				"customer_creation": "always",
				"customer_email": "1xiCJ8M7Pr",
				"customer_update": {
				  "address": "never",
				  "name": "never",
				  "shipping": "auto"
				},
				"discounts": [
				  {
					"coupon": "tOlEXiZKv9",
					"promotion_code": "Xknj8juRnm"
				  }
				],
				"expand": ["ZBxEXz7SN0"],
				"expires_at": 1756067225,
				"invoice_creation": {
				  "enabled": true,
				  "invoice_data": {
					"account_tax_ids": ["dev8vFF6xG"],
					"custom_fields": [
					  {
						"name": "LBlZjJ4gEy",
						"value": "EWoKgkV3fg"
					  }
					],
					"description": "MiePp9LfkQ",
					"footer": "OAELqbYbKV",
					"issuer": {
					  "account": "aqOwDzxnyg",
					  "type": "account"
					},
					"metadata": null,
					"rendering_options": {
					  "amount_tax_display": "exclude_tax"
					}
				  }
				},
				"line_items": [
				  {
					"adjustable_quantity": {
					  "enabled": false,
					  "maximum": 1665059759,
					  "minimum": 905088217
					},
					"dynamic_tax_rates": ["jMMvH8TmQD"],
					"price": "fR6vnvprv8",
					"price_data": {
					  "currency": "euIDO8C4A7",
					  "product": "xilQ2QDVdA",
					  "product_data": {
						"description": "DQECtJEsLI",
						"images": ["gE5K8MOzRc"],
						"metadata": null,
						"name": "ak6UVjXl1B",
						"tax_code": "PzbIHvqWJp"
					  },
					  "recurring": {
						"interval": "day",
						"interval_count": 592739346
					  },
					  "tax_behavior": "inclusive",
					  "unit_amount": 945322526,
					  "unit_amount_decimal": "vkJPCvrn9Q"
					},
					"quantity": 968305911,
					"tax_rates": ["Ts1bPAoT0T"]
				  }
				],
				"locale": "auto",
				"metadata": null,
				"mode": "payment",
				"payment_intent_data": {
				  "application_fee_amount": 2033958571,
				  "capture_method": "manual",
				  "description": "yoalRHw9ZG",
				  "metadata": null,
				  "on_behalf_of": "mpkGzXu3st",
				  "receipt_email": "LxJLYGjJ4r",
				  "setup_future_usage": "off_session",
				  "shipping": {
					"address": {
					  "city": "v6nZI33cUt",
					  "country": "O8MBVcia7c",
					  "line1": "3YghEmysVn",
					  "line2": "CM9x9Jizzu",
					  "postal_code": "1aAilmcYiq",
					  "state": "ILODDWP1IP"
					},
					"carrier": "P8mCJlEq1J",
					"name": "mJYqgRIh3S",
					"phone": "CWAbvZM4Kw",
					"tracking_number": "XGOZIrLZf0"
				  },
				  "statement_descriptor": "JCOo6lU8Fy",
				  "statement_descriptor_suffix": "dtPJwyuc4i",
				  "transfer_data": {
					"amount": 94957585,
					"destination": "LrcNMrJPkO"
				  },
				  "transfer_group": "XKfPQPVhOT"
				},
				"payment_method_collection": "always",
				"payment_method_configuration": "uwYSwIZP4V",
				"payment_method_options": {
				  "acss_debit": {
					"currency": "usd",
					"mandate_options": {
					  "custom_mandate_url": "FZwPtJKktL",
					  "default_for": ["invoice"],
					  "interval_description": "iMgay8S9If",
					  "payment_schedule": "sporadic",
					  "transaction_type": "business"
					},
					"setup_future_usage": "off_session",
					"verification_method": "instant"
				  },
				  "affirm": {
					"setup_future_usage": "none"
				  },
				  "afterpay_clearpay": {
					"setup_future_usage": "none"
				  },
				  "alipay": {
					"setup_future_usage": "none"
				  },
				  "au_becs_debit": {
					"setup_future_usage": "none"
				  },
				  "bacs_debit": {
					"setup_future_usage": "off_session"
				  },
				  "bancontact": {
					"setup_future_usage": "none"
				  },
				  "boleto": {
					"expires_after_days": 953467886,
					"setup_future_usage": "none"
				  },
				  "card": {
					"installments": {
					  "enabled": true
					},
					"request_three_d_secure": "any",
					"setup_future_usage": "on_session",
					"statement_descriptor_suffix_kana": "ZvJtIONyDK",
					"statement_descriptor_suffix_kanji": "Y57zexRcIH"
				  },
				  "cashapp": {
					"setup_future_usage": "off_session"
				  },
				  "customer_balance": {
					"bank_transfer": {
					  "eu_bank_transfer": {
						"country": "mzrVWAjBTc"
					  },
					  "requested_address_types": ["iban"],
					  "type": "gb_bank_transfer"
					},
					"funding_type": "bank_transfer",
					"setup_future_usage": "none"
				  },
				  "eps": {
					"setup_future_usage": "none"
				  },
				  "fpx": {
					"setup_future_usage": "none"
				  },
				  "giropay": {
					"setup_future_usage": "none"
				  },
				  "grabpay": {
					"setup_future_usage": "none"
				  },
				  "ideal": {
					"setup_future_usage": "none"
				  },
				  "klarna": {
					"setup_future_usage": "none"
				  },
				  "konbini": {
					"expires_after_days": 664583520,
					"setup_future_usage": "none"
				  },
				  "link": {
					"setup_future_usage": "none"
				  },
				  "mobilepay": {
					"setup_future_usage": "none"
				  },
				  "oxxo": {
					"expires_after_days": 1925345768,
					"setup_future_usage": "none"
				  },
				  "p24": {
					"setup_future_usage": "none",
					"tos_shown_and_accepted": true
				  },
				  "paynow": {
					"setup_future_usage": "none"
				  },
				  "paypal": {
					"capture_method": "manual",
					"preferred_locale": "cs-CZ",
					"reference": "ulLn2NXA1P",
					"risk_correlation_id": "fj1J6Nux6P",
					"setup_future_usage": "none"
				  },
				  "pix": {
					"expires_after_seconds": 191312234
				  },
				  "revolut_pay": {
					"setup_future_usage": "off_session"
				  },
				  "sepa_debit": {
					"setup_future_usage": "none"
				  },
				  "sofort": {
					"setup_future_usage": "none"
				  },
				  "swish": {
					"reference": "rXJq1EX4rc"
				  },
				  "us_bank_account": {
					"financial_connections": {
					  "permissions": ["ownership"],
					  "prefetch": ["transactions"]
					},
					"setup_future_usage": "none",
					"verification_method": "automatic"
				  },
				  "wechat_pay": {
					"app_id": "9Pu0d1pZ2r",
					"client": "ios",
					"setup_future_usage": "none"
				  }
				},
				"payment_method_types": ["acss_debit"],
				"phone_number_collection": {
				  "enabled": true
				},
				"redirect_on_completion": "never",
				"return_url": "YgIdKykEHC",
				"setup_intent_data": {
				  "description": "U9qFTQnt1W",
				  "metadata": null,
				  "on_behalf_of": "165u5Fvodj"
				},
				"shipping_address_collection": {
				  "allowed_countries": ["AC"]
				},
				"shipping_options": [
				  {
					"shipping_rate": "5PAjqTpMjw",
					"shipping_rate_data": {
					  "delivery_estimate": {
						"maximum": {
						  "unit": "week",
						  "value": 479399576
						},
						"minimum": {
						  "unit": "day",
						  "value": 1640284987
						}
					  },
					  "display_name": "PXozGQQnBA",
					  "fixed_amount": {
						"amount": 2040036333,
						"currency": "KkRL3jvZMO",
						"currency_options": null
					  },
					  "metadata": null,
					  "tax_behavior": "exclusive",
					  "tax_code": "NKSQxYdCfO",
					  "type": "fixed_amount"
					}
				  }
				],
				"submit_type": "donate",
				"subscription_data": {
				  "application_fee_percent": 1.7020678102144877,
				  "billing_cycle_anchor": 1981798554,
				  "default_tax_rates": ["b3jgFBJq4f"],
				  "description": "7mpaD2E0jf",
				  "invoice_settings": {
					"issuer": {
					  "account": "axhiYamJKY",
					  "type": "account"
					}
				  },
				  "metadata": null,
				  "on_behalf_of": "oGsMnSifXV",
				  "proration_behavior": "create_prorations",
				  "transfer_data": {
					"amount_percent": 1.5805719275050356,
					"destination": "wzJ3U1Tyhd"
				  },
				  "trial_end": 606476058,
				  "trial_period_days": 1684102049,
				  "trial_settings": {
					"end_behavior": {
					  "missing_payment_method": "create_invoice"
					}
				  }
				},
				"success_url": "hDTwi34TAz",
				"tax_id_collection": {
				  "enabled": true
				},
				"ui_mode": "hosted"
			}`,
			ResultType: schema.NewNamedType("PostCheckoutSessionsBodyObjectInput").Encode(),
		},
	}

	ndcSchema := createMockSchema(t)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			decoded, err := NewJSONDecoder(ndcSchema, JSONDecodeOptions{}).
				Decode(bytes.NewBuffer([]byte(tc.Input)), tc.ResultType)
			assert.NilError(t, err)
			resultBytes, err := NewJSONEncoder(ndcSchema).Encode(decoded, tc.ResultType)
			assert.NilError(t, err)

			expected, err := NewJSONDecoder(ndcSchema, JSONDecodeOptions{}).
				Decode(bytes.NewBuffer(resultBytes), tc.ResultType)
			assert.NilError(t, err)
			assert.DeepEqual(t, expected, decoded)
		})
	}
}
