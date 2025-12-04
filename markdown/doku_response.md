this is a webhook DOKU of successfull payment:
```
{
  "service": {
    "id": "VIRTUAL_ACCOUNT"
  },
  "acquirer": {
    "id": "BCA"
  },
  "channel": {
    "id": "VIRTUAL_ACCOUNT_BCA"
  },
  "order": {
    "invoice_number": "aj-faizuser2v2h3pk-30-menit-1764770400",
    "amount": 50000
  },
  "virtual_account_info": {
    "virtual_account_number": "1900800000208362"
  },
  "virtual_account_payment": {
    "date": "20251203210101",
    "systrace_number": "116095",
    "reference_number": "64775",
    "channel_code": "",
    "request_id": "048881",
    "identifier": [
      {
        "name": "REQUEST_ID",
        "value": "048881"
      },
      {
        "name": "REFERENCE",
        "value": "64775"
      },
      {
        "name": "CHANNEL_TYPE",
        "value": "6014"
      }
    ]
  },
  "transaction": {
    "status": "SUCCESS",
    "date": "2025-12-03T14:01:01Z",
    "original_request_id": "d7721e1c-5dbe-4399-9d82-8b55918d88cb"
  },
  "additional_info": {
    "origin": {
      "source": "direct",
      "system": "mid-jokul-checkout-system",
      "product": "CHECKOUT",
      "apiFormat": "JOKUL"
    },
    "account": {
      "id": "SAC-7327-1764507463535"
    }
  }
}
```

this is the response body of creating payment link:
```
{
  "message" : [ "SUCCESS" ],
  "response" : {
    "order" : {
      "amount" : "500000",
      "invoice_number" : "aj-faizijui2f-30-menit-1764831449",
      "session_id" : "f03a471b86db493682acd04adbe5c381"
    },
    "payment" : {
      "payment_method_types" : [ "CREDIT_CARD", "OCTO_CLICKS", "QRIS", "PEER_TO_PEER_KREDIVO", "EMONEY_OVO", "JENIUS_PAY", "ONLINE_TO_OFFLINE_ALFA", "VIRTUAL_ACCOUNT_BCA", "ONLINE_TO_OFFLINE_INDOMARET", "VIRTUAL_ACCOUNT_BANK_MANDIRI", "EMONEY_DOKU", "EPAY_BRI", "PEER_TO_PEER_AKULAKU", "EMONEY_LINKAJA", "PEER_TO_PEER_INDODANA", "VIRTUAL_ACCOUNT_BRI", "VIRTUAL_ACCOUNT_BNI", "EMONEY_SHOPEE_PAY", "VIRTUAL_ACCOUNT_BANK_PERMATA", "VIRTUAL_ACCOUNT_DOKU", "VIRTUAL_ACCOUNT_BANK_CIMB", "VIRTUAL_ACCOUNT_BANK_DANAMON", "VIRTUAL_ACCOUNT_BANK_SYARIAH_MANDIRI", "VIRTUAL_ACCOUNT_MAYBANK", "PERMATA_NET", "DANAMON_ONLINE_BANKING", "VIRTUAL_ACCOUNT_BSS", "VIRTUAL_ACCOUNT_SINARMAS", "VIRTUAL_ACCOUNT_BTN", "DIRECT_DEBIT_ALLO", "DIRECT_DEBIT_BRI", "DIRECT_DEBIT_CIMB", "VIRTUAL_ACCOUNT_BNC", "EMONEY_DANA", "KARTU_KREDIT_INDONESIA", "PEER_TO_PEER_BRI_CERIA", "VIRTUAL_ACCOUNT_BPD_BALI", "VIRTUAL_ACCOUNT_BANK_BJB", "DIRECT_DEBIT_MANDIRI" ],
      "payment_due_date" : 60,
      "token_id" : "f03a471b86db493682acd04adbe5c38120255704135732778",
      "url" : "https://staging.doku.com/checkout-link-v2/f03a471b86db493682acd04adbe5c38120255704135732778",
      "expired_date" : "20251204145732",
      "expired_datetime" : "2025-12-04T07:57:32Z"
    },
    "customer" : {
      "email" : "faiz+customer1@aturjadwal.com",
      "name" : "Alice"
    },
    "additional_info" : {
      "origin" : {
        "product" : "CHECKOUT",
        "system" : "mid-jokul-checkout-system",
        "source" : "direct",
        "apiFormat" : "JOKUL"
      },
      "account" : {
        "id" : "SAC-6604-1764297586731"
      }
    },
    "uuid" : 2225251204135732700107182560247753618523,
    "headers" : {
      "request_id" : "4bdaf4b7-af2b-4221-b1eb-85a347aae4ab",
      "signature" : "HMACSHA256=JVGGx4RgePBoIhWIOtUanpUdycMTad8Q+WXAFYemevc=",
      "date" : "2025-12-04T06:57:32Z",
      "client_id" : "BRN-0203-1761932477275"
    }
  }
}
```
