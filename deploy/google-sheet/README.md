# Leads in a Google Sheet

The front door sends each lead to a Google Sheet when a visitor starts a demo, and again when it ends with how. One lead
is one row.

## Set up

1. Make a Google Sheet. Choose **Extensions → Apps Script**.
2. Replace the code with `leads.gs` from this directory.
3. Choose **Deploy → New deployment → Web app**. Set **Execute as: Me** and **Who has access: Anyone**. Choose **Deploy**
   and give the script the access it asks for. Copy the web app address, which ends in `/exec`.
4. Run this on your machine, and paste the address at the first prompt. Press Enter at the second prompt, and the script
   shows a secret once.

   ```bash
   deploy/aws/openshell/put-lead-hook.sh
   ```

5. In the script, choose **Project Settings → Script properties → Add**. Set `SECRET` to the secret from step 4.
6. Upgrade the front door, or restart it. Start a demo, and a row appears in the sheet's `Leads` tab.

## What protects it

The address is public to whoever has it, so the script does nothing unless the request carries the secret. The address and
the secret are stored in AWS Systems Manager as SecureStrings and are on no command line. A cell never has them.

If the sheet cannot be reached, the front door keeps the lead in `leadhook/` beside its other files and sends it later, in
order.
