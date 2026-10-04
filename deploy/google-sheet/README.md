# Leads in a Google Sheet

The front door sends each lead to a Google Sheet when a visitor starts a demo, and again when it ends with how. One lead
is one row.

## Set up

1. Make a Google Sheet. Choose **Extensions → Apps Script**.
2. Replace the code with `leads.gs` from this directory. Run `setup` once and give the script the access it asks for. It
   makes the secret and the `Leads` tab.
3. Choose **Deploy → New deployment → Web app**. Set **Execute as: Me** and **Who has access: Anyone**. Choose **Deploy**.
   Copy the web app address, which ends in `/exec`.
4. In the script, open **Project Settings → Script properties** and copy the value of `SECRET`.
5. Run this on your machine. Paste the address at the first prompt and the secret at the second.

   ```bash
   deploy/aws/openshell/put-lead-hook.sh
   ```

6. Upgrade the front door, or restart it. Start a demo, and a row appears in the sheet's `Leads` tab.

## Mail when someone asks

When a demo starts, the script mails the owner of the script: the name, work email, company and time. It does not mail the
visitor. To send it to another address, add a script property `NOTIFY` with that address. The first time you deploy this
version, Google asks you to allow the script to send mail. Choose **Deploy → Manage deployments → Edit → New version →
Deploy** after pasting `leads.gs`, so the address stays the same.

A mail that cannot be sent, because the day's quota is used, does not lose the lead: its row is still added.

## What protects it

The address is public to whoever has it, so the script does nothing unless the request carries the secret. The address and
the secret are stored in AWS Systems Manager as SecureStrings and are on no command line. A cell never has them.

If the sheet cannot be reached, the front door keeps the lead in `leadhook/` beside its other files and sends it later, in
order.
